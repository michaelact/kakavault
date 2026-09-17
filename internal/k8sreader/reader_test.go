package k8sreader

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFetchSecret_OK(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-secret-variables",
			Namespace: "myrepo-staging",
		},
		Data: map[string][]byte{
			"OPENAI_API_KEY": []byte("sk-abc123"),
			"ENCRYPTION_KEY": []byte("super-secret"),
		},
	}
	client := fake.NewSimpleClientset(secret)
	r := New(client)

	got, err := r.FetchSecret(context.Background(), "myrepo-staging", "backend-secret-variables")
	if err != nil {
		t.Fatalf("FetchSecret() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(FetchSecret()) = %d, want 2", len(got))
	}
	if got["OPENAI_API_KEY"] != "sk-abc123" {
		t.Errorf("got[OPENAI_API_KEY] = %q, want %q", got["OPENAI_API_KEY"], "sk-abc123")
	}
	if got["ENCRYPTION_KEY"] != "super-secret" {
		t.Errorf("got[ENCRYPTION_KEY] = %q, want %q", got["ENCRYPTION_KEY"], "super-secret")
	}
}

func TestFetchSecret_NotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	r := New(client)

	_, err := r.FetchSecret(context.Background(), "myrepo-staging", "does-not-exist")
	if err == nil {
		t.Fatal("FetchSecret() error = nil, want an error for a missing Secret")
	}
}

func TestFetchSecret_EmptySecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-secret-variables", Namespace: "myrepo-staging"},
		Data:       map[string][]byte{},
	}
	client := fake.NewSimpleClientset(secret)
	r := New(client)

	got, err := r.FetchSecret(context.Background(), "myrepo-staging", "empty-secret-variables")
	if err != nil {
		t.Fatalf("FetchSecret() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(FetchSecret()) = %d, want 0", len(got))
	}
}

func TestListNamespaces(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myrepo-staging"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myrepo-prod"}},
	)
	r := New(client)

	got, err := r.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}

	want := map[string]bool{"myrepo-staging": true, "myrepo-prod": true}
	if len(got) != len(want) {
		t.Fatalf("len(ListNamespaces()) = %d, want %d", len(got), len(want))
	}
	for _, ns := range got {
		if !want[ns] {
			t.Errorf("unexpected namespace %q in result", ns)
		}
	}
}

func TestListNamespaces_None(t *testing.T) {
	client := fake.NewSimpleClientset()
	r := New(client)

	got, err := r.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(ListNamespaces()) = %d, want 0", len(got))
	}
}

func TestListSecrets(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "backend-secret-variables", Namespace: "myrepo-staging"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "default-token-abc", Namespace: "myrepo-staging"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ui-secret-variables", Namespace: "other-namespace"}},
	)
	r := New(client)

	got, err := r.ListSecrets(context.Background(), "myrepo-staging")
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}

	want := map[string]bool{"backend-secret-variables": true, "default-token-abc": true}
	if len(got) != len(want) {
		t.Fatalf("len(ListSecrets()) = %d, want %d; got %v", len(got), len(want), got)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected secret %q in result", name)
		}
	}
}
