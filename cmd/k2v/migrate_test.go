package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const testConfigYAML = `
namespace_pattern: "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$"
secret_name_pattern: "^(?P<application>.+)-secret-variables$"
vault:
  mount: default
  kv_version: 2
classification:
  rules:
    - pattern: "_API_KEY$"
      classification: third-party
    - pattern: ".*"
      classification: internal
`

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, []byte(testConfigYAML), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

func fakeK8sClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backend-secret-variables", Namespace: "myrepo-staging"},
		Data: map[string][]byte{
			"OPENAI_API_KEY": []byte("sk-abc"),
			"ENCRYPTION_KEY": []byte("enc-value"),
		},
	}
	return fake.NewSimpleClientset(secret)
}

func fakeK8sClientMultiNamespace(t *testing.T) kubernetes.Interface {
	t.Helper()
	return fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myrepo-staging"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myrepo-prod"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "backend-secret-variables", Namespace: "myrepo-staging"},
			Data:       map[string][]byte{"OPENAI_API_KEY": []byte("sk-abc")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ui-secret-variables", Namespace: "myrepo-prod"},
			Data:       map[string][]byte{"ENCRYPTION_KEY": []byte("enc-value")},
		},
		&corev1.Secret{
			// Doesn't match secret_name_pattern — must be skipped.
			ObjectMeta: metav1.ObjectMeta{Name: "default-token-abc", Namespace: "myrepo-staging"},
			Data:       map[string][]byte{"token": []byte("x")},
		},
	)
}

func TestRunMigrate_DryRun_NoApplyFlag(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout, stderr bytes.Buffer

	code := runMigrate(migrateArgs{
		namespace:  "myrepo-staging",
		secretName: "backend-secret-variables",
		configPath: configPath,
		apply:      false,
	}, fakeK8sClient(t), nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runMigrate() exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "OPENAI_API_KEY") || !strings.Contains(out, "third-party") {
		t.Errorf("dry-run output missing expected content; got:\n%s", out)
	}
	if !strings.Contains(out, "ENCRYPTION_KEY") || !strings.Contains(out, "internal") {
		t.Errorf("dry-run output missing expected content; got:\n%s", out)
	}
}

func TestRunMigrate_NamespaceMismatch_ReturnsNonZero(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout, stderr bytes.Buffer

	code := runMigrate(migrateArgs{
		namespace:  "badnamespace",
		secretName: "backend-secret-variables",
		configPath: configPath,
		apply:      false,
	}, fakeK8sClient(t), nil, &stdout, &stderr)

	if code == 0 {
		t.Fatal("runMigrate() exit code = 0, want non-zero for a namespace pattern mismatch")
	}
	if !strings.Contains(stderr.String(), "does not match") {
		t.Errorf("stderr should explain the mismatch; got:\n%s", stderr.String())
	}
}

func TestRunMigrate_ConfigNotFound_ReturnsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runMigrate(migrateArgs{
		namespace:  "myrepo-staging",
		secretName: "backend-secret-variables",
		configPath: "/nonexistent/config.yaml",
		apply:      false,
	}, fakeK8sClient(t), nil, &stdout, &stderr)

	if code == 0 {
		t.Fatal("runMigrate() exit code = 0, want non-zero for a missing config file")
	}
}

func TestRunMigrate_Apply_UsesVaultWriter(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout, stderr bytes.Buffer
	writer := newFakeVaultWriter()

	code := runMigrate(migrateArgs{
		namespace:  "myrepo-staging",
		secretName: "backend-secret-variables",
		configPath: configPath,
		apply:      true,
	}, fakeK8sClient(t), writer, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runMigrate() exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	if len(writer.data) != 2 {
		t.Errorf("writer.data has %d entries, want 2", len(writer.data))
	}
	out := stdout.String()
	if !strings.Contains(out, "written") {
		t.Errorf("apply output should show \"written\" status; got:\n%s", out)
	}
}

func TestRunMigrate_All_DiscoversAndMigratesEachTarget(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout, stderr bytes.Buffer
	writer := newFakeVaultWriter()

	code := runMigrate(migrateArgs{
		configPath: configPath,
		apply:      true,
		all:        true,
	}, fakeK8sClientMultiNamespace(t), writer, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runMigrate() exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "myrepo-staging / backend-secret-variables") {
		t.Errorf("output should show the staging target header; got:\n%s", out)
	}
	if !strings.Contains(out, "myrepo-prod / ui-secret-variables") {
		t.Errorf("output should show the prod target header; got:\n%s", out)
	}
	if strings.Contains(out, "kube-system") {
		t.Errorf("output should never mention kube-system; got:\n%s", out)
	}

	if len(writer.data) != 2 {
		t.Errorf("writer.data has %d entries, want 2 (one per target's single key)", len(writer.data))
	}
}

func TestRunMigrate_All_NoMatches_ReturnsZero(t *testing.T) {
	configPath := writeTestConfig(t)
	var stdout, stderr bytes.Buffer

	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "nohyphen"}},
	)

	code := runMigrate(migrateArgs{
		configPath: configPath,
		apply:      false,
		all:        true,
	}, client, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runMigrate() exit code = %d, want 0 for zero matches; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "nothing to do") {
		t.Errorf("output should explain nothing matched; got:\n%s", stdout.String())
	}
}
