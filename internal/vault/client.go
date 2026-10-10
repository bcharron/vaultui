// Package vault wraps the official Vault API client with the operations
// vaultui needs: listing mounts, reading mount configs, roles, policies,
// and browsing KV secrets.
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/vault/api"
)

// NewClient builds a Vault client configured from the environment
// (VAULT_ADDR, VAULT_NAMESPACE, VAULT_CACERT, ... as handled by
// api.DefaultConfig) with the token taken from VAULT_TOKEN or, failing
// that, from ~/.vault-token as written by `vault login`.
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
		token, err = readTokenFile()
		if err != nil {
			return nil, err
		}
	}
	if token == "" {
		return nil, fmt.Errorf("no Vault token found; set VAULT_TOKEN or run `vault login`")
	}
	c.SetToken(token)
	return c, nil
}

func readTokenFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil //nolint:nilerr // fall through, caller reports "no token"
	}
	b, err := os.ReadFile(filepath.Join(home, ".vault-token"))
	if err != nil {
		return "", nil //nolint:nilerr // fall through, caller reports "no token"
	}
	return strings.TrimSpace(string(b)), nil
}
