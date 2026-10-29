# vaultui

A terminal UI for HashiCorp Vault. Browse secret engines, inspect their
configs and roles, walk KV secret trees, and read ACL policies.

## Requirements

- Go 1.21+
- Vault address and token (see below)

## Build & run

```sh
go build -o vaultui .
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN=...
./vaultui
```

The token is resolved from `VAULT_TOKEN`, falling back to `~/.vault-token`
(as written by `vault login`). All other `VAULT_*` environment variables
(`VAULT_NAMESPACE`, `VAULT_CACERT`, ...) are honored via the official
`hashicorp/vault/api` client.

## Usage

Navigate with the arrow keys or `j`/`k`, open with `enter`, go back with
`esc`, refresh the current page with `r`, quit with `q`.

```
Home
├── Secret Engines ........ list of mounts (path, type, description)
│   └── Engine detail
│       ├── Config ........ sys/mounts/{path}/tune (TTLs, options, ...)
│       ├── Engine Config . engine-owned config (aws config/root and
│       │                   config/lease, gcp config)
│       ├── Roles ......... roles/keys per engine type (database, pki,
│       │                   transit, ...) with drill-in detail; AWS
│       │                   adds a Static Roles tab, and AWS/GCP roles
│       │                   show their credential/secret type inline
│       ├── Rolesets ...... GCP splits role-like objects into Rolesets,
│       │                   Static Accounts, and Impersonated Accounts
│       │                   tabs
│       └── Browse ........ KV v1/v2 key listing (KV engines)
│           └── KV browser  directory tree navigation
│               └── Secret  key/value data plus v2 metadata
├── ACL Policies ......... list + policy document viewer
├── Auth Methods ......... enabled auth mounts (jwt, oidc, approle, ...)
│   └── Auth detail
│       ├── Config ........ sys/auth/{path}/tune
│       └── Roles ......... roles/users/teams/groups per method type,
│                           with inline "-> policies" column and drill-in
│                           detail; press the number next to a linked
│                           policy to jump straight to it
├── Token Info ........... lookup-self (policies, TTLs, ...) plus a
│                          capabilities check: type a path, see what
│                          your token can do on it
├── Server Status ........ sys/seal-status + sys/health (seal state,
│                          version, cluster identity, HA)
├── Leases ............... tree browser over sys/leases/lookup with
│                          drill-in to each lease's times, TTL, and
│                          renewable flag
└── Identity ............. entities and groups (two tabs), each with
                           drill-in detail and linked-policy jumps
```

On a KV secret in the browser, `v` opens its version history (KV v2):
each version's creation time and state, with drill-in to that version's
data. vaultui is read-only: no page issues writes to Vault.

## Development

```sh
# end-to-end test against a local dev Vault (skipped when absent):
vault server -dev -dev-root-token-id=root -dev-listen-address=127.0.0.1:8200 &
VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root scripts/dev-fixture.sh
VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root go test ./...
```

The test only runs against loopback addresses and skips otherwise, so
`go test ./...` is safe on any machine. `scripts/dev-fixture.sh` seeds
the data the test expects (KV secrets and versions, transit keys, a PKI
role, policies, a jwt auth login, identity entities and groups); it is
idempotent and safe to re-run. The AWS and GCP engines and their sample
roles are enabled by the test itself.

## Layout

- `internal/vault/` — thin wrappers over the Vault API: mounts, auth
  methods, tune configs, roles, policies, KV v1/v2 listing and reads.
- `internal/tui/` — Bubble Tea pages over a navigation stack: home,
  engines, engine detail (tabbed), auth methods, auth detail (tabbed),
  KV browser, generic detail viewer, policies.
