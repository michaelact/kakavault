# kakavault

Kubernetes Secret to HashiCorp Vault.

Migrate a Kubernetes `Secret`'s keys into HashiCorp Vault's KV-v2 store,
one Vault secret per key, split by a config-driven classification
(e.g. `internal` vs `third-party`).

This is a one-shot migration tool, not an ongoing sync — see
`docs/superpowers/specs/2026-09-17-kakavault-design.md` for the full design
and why (short version: once an app's secrets are in Vault, its
manifests should read from Vault directly and the K8s Secret goes
away).

## Install

```bash
go install github.com/michaelact/kakavault/cmd/kakavault@latest
```

## Usage

```bash
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN=...

# Dry run — prints what would be written, writes nothing
kakavault migrate --namespace myrepo-staging --secret backend-secret-variables --config config.yaml

# Actually write to Vault, then read back and verify each key
kakavault migrate --namespace myrepo-staging --secret backend-secret-variables --config config.yaml --apply

# --namespace with no --secret: discover every Secret in that one
# namespace matching secret_name_pattern, and migrate each one.
kakavault migrate --namespace myrepo-staging --config config.yaml
kakavault migrate --namespace myrepo-staging --config config.yaml --apply

# --all: discover every namespace/Secret pair matching the config's
# namespace_pattern/secret_name_pattern across the whole cluster. Skips
# kube-system/kube-public/kube-node-lease. Mutually exclusive with
# --namespace/--secret.
kakavault migrate --all --config config.yaml
kakavault migrate --all --config config.yaml --apply
```

See `examples/config.example.yaml` for a config template.

## Path convention

```
<mount>/<repo>/<environment>/<application>/<classification>
```

One Vault secret per classification tier — every key sharing that
classification (e.g. all `internal` keys, all `third-party` keys) lives
as a field in that one secret, not as its own separate Vault secret.

`repo`/`environment` come from parsing the namespace name; `application`
comes from parsing the Secret name — both via configurable regexes, so
this isn't tied to any one naming convention.

## Development

```bash
go build ./...
go vet ./...
go test ./...                        # unit tests, no external deps
go test -tags=integration ./...      # + end-to-end test, needs `vault` on PATH
```
