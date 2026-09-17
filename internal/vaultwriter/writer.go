// Package vaultwriter writes and reads a classification tier's Vault
// KV-v2 secret — one secret per tier, holding every key sharing that
// classification as top-level fields.
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

// Write puts values at subpath (relative to the mount) as a single KV-v2
// secret, one field per map entry.
func (w *Writer) Write(ctx context.Context, subpath string, values map[string]string) error {
	data := make(map[string]interface{}, len(values))
	for k, v := range values {
		data[k] = v
	}

	_, err := w.client.KVv2(w.mount).Put(ctx, subpath, data)
	if err != nil {
		return fmt.Errorf("write %s/%s: %w", w.mount, subpath, err)
	}
	return nil
}

// Read fetches subpath's full secret document. found is false (with a
// nil error) when the secret doesn't exist — that's an expected outcome
// for verification/dry-run flows, not a failure.
func (w *Writer) Read(ctx context.Context, subpath string) (values map[string]string, found bool, err error) {
	secret, err := w.client.KVv2(w.mount).Get(ctx, subpath)
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s/%s: %w", w.mount, subpath, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, false, nil
	}

	values = make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		s, ok := v.(string)
		if !ok {
			return nil, false, fmt.Errorf("read %s/%s: field %q is not a string", w.mount, subpath, k)
		}
		values[k] = s
	}
	return values, true, nil
}

func isNotFound(err error) bool {
	if errors.Is(err, vaultapi.ErrSecretNotFound) {
		return true
	}
	var respErr *vaultapi.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == 404
}
