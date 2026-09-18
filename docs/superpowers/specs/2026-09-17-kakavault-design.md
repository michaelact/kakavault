# kakavault — Design

## Problem

Many orgs store application secrets as Kubernetes `Secret` objects,
seeded via encrypted Helm values files or similar. Migrating to
HashiCorp Vault as the source of truth for secrets means manually
copying dozens of keys per app from a K8s Secret into the right Vault
KV path, split by classification — slow and error-prone. This tool
automates that one-time migration per app.

Not in scope: ongoing sync, drift correction, or anything that treats
the K8s Secret as authoritative after migration. Once an app is
migrated, its manifests should read from Vault (via ESO or Vault
Agent) and the K8s Secret goes away.

## Prior art

- **External Secrets Operator's `PushSecret`** does K8s Secret → Vault
  (or other backends), officially maintained, widely adopted. It's
  generic 1:1 key copying, though — no per-key classification, so it
  can't split one Secret's keys across `internal`/`third-party`
  subpaths on its own. Not used here to keep this a zero-dependency
  CLI (no requirement that ESO is installed), but if ESO is already
  running somewhere, generating `PushSecret` CRDs from this tool's
  classification output is a reasonable future extension.
- **danieldonoghue/vault-sync-operator** — a smaller, Deployment
  annotation-triggered K8s→Vault operator. Different trigger model
  (live controller vs. one-shot CLI) and no classification concept.

## Path convention

```
<mount>/<repo>/<environment>/<application>/<classification>
```

Example: namespace `myrepo-staging`, Secret
`backend-secret-variables` →
`default/myrepo/staging/backend/internal` (holding `ENCRYPTION_KEY`
and every other key classified `internal`) and
`default/myrepo/staging/backend/third-party` (holding `OPENAI_API_KEY`
and every other key classified `third-party`).

`repo` and `environment` come from parsing the namespace name;
`application` comes from parsing the Secret name. Both patterns are
configurable (see Config below) so the tool isn't hardcoded to any one
org's naming convention.

One Vault KV-v2 secret per classification tier, not per key — every
key sharing a classification is a field in that one secret. This means
a Vault ACL policy grants/denies access at the classification level
(matching how access is actually assigned — e.g. a cloud/ITTS team
role covering all `third-party` credentials for an app), not per
individual variable.

## Classification

Per-key classification (`internal` vs `third-party`) can't be reliably
inferred from key name alone (`AUTH_SERVICE_DATABASE_URL` looks
internal by prefix but is a database URL; `SEARCH_MASTER_PASSWORD`
is ambiguous without knowing if that's an in-house or external
service). So classification is config-driven:

1. An ordered list of `pattern`/`classification` rules (first match
   wins, regex against the key name).
2. An `overrides` map (key name → classification) that always wins
   over the pattern rules, for the keys the ruleset gets wrong.

## Architecture

Single Go binary, one command: `kakavault migrate`. No server, no
daemon, no CRDs, no in-cluster component.

```
load config → fetch Secret(s) (client-go; --all/--namespace-only
  discover targets first via discover.Find/FindInNamespace)
  → parse repo/environment (namespace pattern) + application
  (secret-name pattern) → classify each key → build per-classification
  Vault paths → group keys by path
  → dry-run report (default)
  → --apply: one write per group, then read back and verify each key
  → summary report + exit code
```

### Components

| Component | Responsibility |
|---|---|
| `config` | Load + validate the YAML config (patterns, Vault mount/kv_version, classification rules/overrides) |
| `k8s reader` | `client-go`: `GetSecret`, plus `ListNamespaces`/`ListSecrets` for discovery |
| `discover` | Finds every (namespace, Secret) pair matching the config's patterns, cluster-wide or within one namespace |
| `classifier` | Applies config rules + overrides to a key name → classification |
| `path resolver` | Runs namespace/secret-name regexes, builds the per-classification-tier Vault path |
| `vault writer` | `hashicorp/vault/api`, one KV-v2 write per classification tier (all its keys as fields); read-back verify after `--apply` |
| `reporter` | Table: key → classification → Vault path → status (planned/written/failed) |

## Config

YAML, one file per org/team:

```yaml
namespace_pattern: "^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$"
secret_name_pattern: "^(?P<application>.+)-secret-variables$"

vault:
  mount: default
  kv_version: 2

classification:
  rules:
    - pattern: "(API_KEY|PASSWORD|SECRET_ACCESS_KEY|_DB_URL|_URL)$"
      classification: third-party
    - pattern: ".*"
      classification: internal
  overrides:
    SOME_INTERNAL_SERVICE_KEY: internal
    SOME_DB_URL: third-party
```

## CLI

```
kakavault migrate --namespace myrepo-staging --secret backend-secret-variables --config config.yaml
kakavault migrate --namespace myrepo-staging --config config.yaml                    # every matching Secret in that namespace
kakavault migrate --all --config config.yaml                                         # every matching namespace/Secret in the cluster
```

- No `--apply` flag: dry-run, prints the planned table, writes nothing.
- `--apply`: performs the writes, then reads each one back to verify,
  then prints the summary table with final status per key.
- Vault auth: `VAULT_TOKEN` env var (standard Vault CLI convention,
  read automatically by the official SDK — no auth code needed in the
  tool itself). `VAULT_ADDR`/`VAULT_CACERT` likewise standard env vars.

## Error handling

- Namespace or Secret name doesn't match its configured pattern → hard
  fail before any writes; nothing can be built without repo/environment
  or application.
- Vault auth/permission failure on the *first* write → fail fast, stop
  the run (wrong token isn't going to start working for key #2).
- An individual key's write fails (e.g. mid-run network blip) → don't
  abort the whole run; continue with remaining keys, collect the
  failure, report it in the final table, and exit non-zero if anything
  failed.
- Reruns are safe: writing the same value again is a no-op in effect
  (Vault KV-v2 just adds a version). No local state file is kept —
  Vault itself is the source of truth for what's already migrated.

## Testing

- Unit: classifier (rule order + override precedence), path resolver
  (regex edge cases — no match, hyphens inside the repo name itself),
  config validation (bad regex, missing required fields).
- Integration: `vault server -dev` + `client-go` fake clientset, full
  `migrate --apply` run against a fixture Secret, assert the resulting
  Vault paths/values.

## Out of scope (explicitly)

- Ongoing/continuous sync — see Problem section.
- Deleting or mutating the source K8s Secret.
- Any Vault policy/auth backend setup — provisioning ACL policies or
  login methods is a separate concern, not this tool's job.
- Anything beyond `namespace_pattern`/`secret_name_pattern` filtering for
  discovery (`--all`) — e.g. label selectors, exclude lists beyond the
  hardcoded kube-system/kube-public/kube-node-lease skip. The two regex
  patterns are the only scoping mechanism.
