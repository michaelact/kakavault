//go:build integration

package migrate_test

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/michaelact/kakavault/internal/config"
	"github.com/michaelact/kakavault/internal/migrate"
	"github.com/michaelact/kakavault/internal/vaultwriter"
)

const (
	devVaultAddr  = "http://127.0.0.1:8299"
	devVaultToken = "integration-test-root-token"
)

func startDevVault(t *testing.T) {
	t.Helper()

	cmd := exec.Command("vault", "server", "-dev",
		"-dev-root-token-id="+devVaultToken,
		"-dev-listen-address=127.0.0.1:8299")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start vault dev server: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(devVaultAddr + "/v1/sys/health")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("vault dev server did not become healthy within 10s")
}

func TestMigrate_EndToEnd_RealVault(t *testing.T) {
	startDevVault(t)

	vaultCfg := vaultapi.DefaultConfig()
	vaultCfg.Address = devVaultAddr
	client, err := vaultapi.NewClient(vaultCfg)
	if err != nil {
		t.Fatalf("vaultapi.NewClient() error = %v", err)
	}
	client.SetToken(devVaultToken)

	// The dev server auto-mounts "secret" as a KV-v2 engine, not "default".
	writer := vaultwriter.New(client, "secret")

	cfg := &config.Config{
		NamespacePattern:  `^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$`,
		SecretNamePattern: `^(?P<application>.+)-secret-variables$`,
		Vault:             config.VaultConfig{Mount: "secret", KVVersion: 2},
		Classification: config.Classification{
			Rules: []config.ClassificationRule{
				{Pattern: "_API_KEY$", Classification: "third-party"},
				{Pattern: ".*", Classification: "internal"},
			},
		},
	}

	secretData := map[string]string{
		"OPENAI_API_KEY": "sk-integration-test",
		"ENCRYPTION_KEY": "integration-enc-value",
	}

	items, err := migrate.Plan(cfg, secretData, "myrepo-staging", "backend-secret-variables")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	results := migrate.Apply(context.Background(), writer, items, secretData)
	for _, r := range results {
		if r.Status != "written" {
			t.Errorf("key %s: Status = %q, want %q (err=%v)", r.Key, r.Status, "written", r.Err)
		}
	}

	// Confirm directly against Vault, independent of Apply's own
	// read-back, that both keys landed in their classification tier's
	// shared secret.
	thirdParty, found, err := writer.Read(context.Background(), "myrepo/staging/backend/third-party")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !found || thirdParty["OPENAI_API_KEY"] != "sk-integration-test" {
		t.Errorf("third-party: found=%v OPENAI_API_KEY=%q, want found=true value=%q", found, thirdParty["OPENAI_API_KEY"], "sk-integration-test")
	}

	internal, found, err := writer.Read(context.Background(), "myrepo/staging/backend/internal")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !found || internal["ENCRYPTION_KEY"] != "integration-enc-value" {
		t.Errorf("internal: found=%v ENCRYPTION_KEY=%q, want found=true value=%q", found, internal["ENCRYPTION_KEY"], "integration-enc-value")
	}
}
