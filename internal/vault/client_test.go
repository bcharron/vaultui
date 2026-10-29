package vault

import (
	"os"
	"path/filepath"
	"testing"

	homedir "github.com/mitchellh/go-homedir"
)

// resetHome points the token helper machinery at an isolated fake home
// directory (config file, ~/.vault-token), both via the HOME variable and
// by clearing mitchellh/go-homedir's cached value.
func resetHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("VAULT_ADDR", "https://127.0.0.1:8200")
	t.Setenv("VAULT_TOKEN", "")
	homedir.Reset()
	t.Cleanup(homedir.Reset)
}

func writeHelperConfig(t *testing.T, home, helper string) {
	t.Helper()
	contents := "token_helper = " + `"` + helper + `"` + "\n"
	if err := os.WriteFile(filepath.Join(home, ".vault"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNewClientTokenFromExternalHelper verifies that vaultui consults an
// external token helper configured in ~/.vault, the same way the vault CLI
// does. /bin/echo replies "get" when invoked with the get operation, so a
// client token of "get" proves the helper was executed.
func TestNewClientTokenFromExternalHelper(t *testing.T) {
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skip("/bin/echo not available")
	}
	home := t.TempDir()
	resetHome(t, home)
	writeHelperConfig(t, home, "/bin/echo")

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.Token(); got != "get" {
		t.Fatalf("token = %q, want %q (helper output)", got, "get")
	}
}

// TestNewClientEnvTokenPrecedence verifies VAULT_TOKEN still wins over the
// token helper, matching the vault CLI.
func TestNewClientEnvTokenPrecedence(t *testing.T) {
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skip("/bin/echo not available")
	}
	home := t.TempDir()
	resetHome(t, home)
	writeHelperConfig(t, home, "/bin/echo")
	t.Setenv("VAULT_TOKEN", "env-token")

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.Token(); got != "env-token" {
		t.Fatalf("token = %q, want %q", got, "env-token")
	}
}

// TestNewClientNoToken verifies the error message when neither VAULT_TOKEN
// nor a token helper provides a token.
func TestNewClientNoToken(t *testing.T) {
	resetHome(t, t.TempDir())

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "no Vault token found"
	if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("error = %q, want prefix %q", got, want)
	}
}
