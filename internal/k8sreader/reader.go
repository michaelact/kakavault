// Package k8sreader fetches a single Kubernetes Secret's data as plain
// strings (already base64-decoded by client-go).
package k8sreader

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Reader fetches Secrets through a kubernetes.Interface, so tests can
// substitute k8s.io/client-go/kubernetes/fake.
type Reader struct {
	client kubernetes.Interface
}

// New wraps an existing Kubernetes client. Building that client (from a
// kubeconfig or in-cluster config) is the caller's job — cmd/k2v does it.
func New(client kubernetes.Interface) *Reader {
	return &Reader{client: client}
}

// FetchSecret returns the named Secret's data as a map of key -> decoded
// string value.
func (r *Reader) FetchSecret(ctx context.Context, namespace, name string) (map[string]string, error) {
	secret, err := r.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}

	result := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// ListNamespaces returns every namespace's name in the cluster.
func (r *Reader) ListNamespaces(ctx context.Context) ([]string, error) {
	list, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// ListSecrets returns every Secret's name in namespace.
func (r *Reader) ListSecrets(ctx context.Context, namespace string) ([]string, error) {
	list, err := r.client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list secrets in %s: %w", namespace, err)
	}

	names := make([]string, 0, len(list.Items))
	for _, s := range list.Items {
		names = append(names, s.Name)
	}
	return names, nil
}
