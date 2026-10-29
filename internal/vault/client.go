// Package vault wraps the official Vault API client with the operations
// vaultui needs: listing mounts, reading mount configs, roles, policies,
// and browsing KV secrets.
package vault

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/cliconfig"
)

// NewClient builds a Vault client configured from the environment
// (VAULT_ADDR, VAULT_NAMESPACE, VAULT_CACERT, ... as handled by
// api.DefaultConfig). The token is resolved the same way the vault CLI
// does: VAULT_TOKEN first, then the configured token helper
// (token_helper in ~/.vault, or $VAULT_CONFIG_PATH), whose default
// reads ~/.vault-token as written by `vault login`.
func NewClient() (*api.Client, error) {
	cfg := api.DefaultConfig()
	if cfg.Address == "" || cfg.Address == "https://127.0.0.1:8200" {
		if os.Getenv("VAULT_ADDR") == "" {
			return nil, fmt.Errorf("VAULT_ADDR is not set; export it (e.g. https://vault.example.com:8200)")
		}
	}
	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		helper, err := cliconfig.DefaultTokenHelper()
		if err != nil {
			return nil, fmt.Errorf("resolving token helper: %w", err)
		}
		token, err = helper.Get()
		if err != nil {
			return nil, fmt.Errorf("getting token from token helper %s: %w", helper.Path(), err)
		}
		token = strings.TrimSpace(token)
	}
	if token == "" {
		return nil, fmt.Errorf("no Vault token found; set VAULT_TOKEN or run `vault login`")
	}
	c.SetToken(token)
	return c, nil
}
