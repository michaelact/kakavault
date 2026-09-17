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
