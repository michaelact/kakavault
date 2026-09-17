package vaultwriter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *vaultapi.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := vaultapi.DefaultConfig()
	cfg.Address = server.URL
	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		t.Fatalf("vaultapi.NewClient() error = %v", err)
	}
	client.SetToken("test-token")
	return client
}

func TestWrite_OK(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"version": 1},
		})
	})

	writer := New(client, "default")
	err := writer.Write(context.Background(), "myrepo/staging/backend/internal", map[string]string{
		"ENCRYPTION_KEY": "super-secret",
		"HASH_SECRET":    "another-secret",
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	wantPath := "/v1/default/data/myrepo/staging/backend/internal"
	if gotPath != wantPath {
		t.Errorf("request path = %q, want %q", gotPath, wantPath)
	}

	data, ok := gotBody["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("request body has no \"data\" object: %+v", gotBody)
	}
	if data["ENCRYPTION_KEY"] != "super-secret" {
		t.Errorf("request body data.ENCRYPTION_KEY = %v, want %q", data["ENCRYPTION_KEY"], "super-secret")
	}
	if data["HASH_SECRET"] != "another-secret" {
		t.Errorf("request body data.HASH_SECRET = %v, want %q", data["HASH_SECRET"], "another-secret")
	}
}

func TestRead_Found(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"ENCRYPTION_KEY": "super-secret",
					"HASH_SECRET":    "another-secret",
				},
				"metadata": map[string]interface{}{"version": 1},
			},
		})
	})

	writer := New(client, "default")
	values, found, err := writer.Read(context.Background(), "myrepo/staging/backend/internal")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !found {
		t.Fatal("Read() found = false, want true")
	}
	if values["ENCRYPTION_KEY"] != "super-secret" {
		t.Errorf("values[ENCRYPTION_KEY] = %q, want %q", values["ENCRYPTION_KEY"], "super-secret")
	}
	if values["HASH_SECRET"] != "another-secret" {
		t.Errorf("values[HASH_SECRET] = %q, want %q", values["HASH_SECRET"], "another-secret")
	}
}

func TestRead_NotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"errors": []string{}})
	})

	writer := New(client, "default")
	_, found, err := writer.Read(context.Background(), "myrepo/staging/backend/internal")
	if err != nil {
		t.Fatalf("Read() error = %v, want nil (not-found is found=false, not an error)", err)
	}
	if found {
		t.Error("Read() found = true, want false")
	}
}

func TestWrite_ServerError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"errors": []string{"internal error"}})
	})

	writer := New(client, "default")
	err := writer.Write(context.Background(), "myrepo/staging/backend/internal", map[string]string{"KEY": "value"})
	if err == nil {
		t.Fatal("Write() error = nil, want an error on a 500 response")
	}
}
