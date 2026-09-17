// Package vaultwriter writes and reads single keys against a Vault KV-v2
// mount, one Vault secret per key (value shape {"value": "..."}).
package vaultwriter

import (
	"context"
	"errors"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
)

// Writer writes/reads KV-v2 secrets under a single fixed mount.
type Writer struct {
	client *vaultapi.Client
	mount  string
}

// New wraps an already-configured Vault client (VAULT_ADDR/VAULT_TOKEN/
// VAULT_CACERT are read from the environment by vaultapi.NewClient in the
// caller — this package takes no auth dependency of its own).
func New(client *vaultapi.Client, mount string) *Writer {
	return &Writer{client: client, mount: mount}
}

// Write puts value at subpath (relative to the mount) as a KV-v2 secret
// with a single "value" field.
func (w *Writer) Write(ctx context.Context, subpath, value string) error {
	_, err := w.client.KVv2(w.mount).Put(ctx, subpath, map[string]interface{}{
		"value": value,
	})
	if err != nil {
		return fmt.Errorf("write %s/%s: %w", w.mount, subpath, err)
	}
	return nil
}

// Read fetches subpath's "value" field. found is false (with a nil error)
// when the secret doesn't exist — that's an expected outcome for
// verification/dry-run flows, not a failure.
func (w *Writer) Read(ctx context.Context, subpath string) (value string, found bool, err error) {
	secret, err := w.client.KVv2(w.mount).Get(ctx, subpath)
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read %s/%s: %w", w.mount, subpath, err)
	}
	if secret == nil || secret.Data == nil {
		return "", false, nil
	}

	v, ok := secret.Data["value"].(string)
	if !ok {
		return "", false, fmt.Errorf("read %s/%s: \"value\" field missing or not a string", w.mount, subpath)
	}
	return v, true, nil
}

func isNotFound(err error) bool {
	if errors.Is(err, vaultapi.ErrSecretNotFound) {
		return true
	}
	var respErr *vaultapi.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == 404
}
