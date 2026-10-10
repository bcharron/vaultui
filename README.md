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
│       ├── Roles ......... roles/keys per engine type (database, pki,
│       │                   transit, aws, ...) with drill-in detail
│       └── Browse ........ KV v1/v2 key listing (KV engines)
│           └── KV browser  directory tree navigation
│               └── Secret  key/value data plus v2 metadata
├── ACL Policies ......... list + policy document viewer
└── Auth Methods ......... enabled auth mounts (jwt, oidc, approle, ...)
    └── Auth detail
        ├── Config ........ sys/auth/{path}/tune
        └── Roles ......... roles/users/teams/groups per method type,
                            with drill-in detail
```

## Development

```sh
# end-to-end test against a local dev Vault (skipped when absent):
vault server -dev -dev-root-token-id=root -dev-listen-address=127.0.0.1:8200 &
VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root go test ./...
```

The test only runs against loopback addresses and skips otherwise, so
`go test ./...` is safe on any machine.

## Layout

- `internal/vault/` — thin wrappers over the Vault API: mounts, auth
  methods, tune configs, roles, policies, KV v1/v2 listing and reads.
- `internal/tui/` — Bubble Tea pages over a navigation stack: home,
  engines, engine detail (tabbed), auth methods, auth detail (tabbed),
  KV browser, generic detail viewer, policies.
