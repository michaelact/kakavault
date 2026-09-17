package main

import "context"

// fakeVaultWriter is an in-memory migrate.VaultWriter for cmd/k2v's own
// tests — separate from internal/migrate's fakeWriter since Go test
// helpers aren't exported across packages.
type fakeVaultWriter struct {
	data map[string]string
}

func newFakeVaultWriter() *fakeVaultWriter {
	return &fakeVaultWriter{data: map[string]string{}}
}

func (f *fakeVaultWriter) Write(ctx context.Context, subpath, value string) error {
	f.data[subpath] = value
	return nil
}

func (f *fakeVaultWriter) Read(ctx context.Context, subpath string) (string, bool, error) {
	v, ok := f.data[subpath]
	return v, ok, nil
}
