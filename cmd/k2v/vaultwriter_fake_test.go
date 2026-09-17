package main

import "context"

// fakeVaultWriter is an in-memory migrate.VaultWriter for cmd/k2v's own
// tests — separate from internal/migrate's fakeWriter since Go test
// helpers aren't exported across packages. data is keyed by subpath,
// each value holding every key written to that classification tier.
type fakeVaultWriter struct {
	data map[string]map[string]string
}

func newFakeVaultWriter() *fakeVaultWriter {
	return &fakeVaultWriter{data: map[string]map[string]string{}}
}

func (f *fakeVaultWriter) Write(ctx context.Context, subpath string, values map[string]string) error {
	f.data[subpath] = values
	return nil
}

func (f *fakeVaultWriter) Read(ctx context.Context, subpath string) (map[string]string, bool, error) {
	v, ok := f.data[subpath]
	return v, ok, nil
}
