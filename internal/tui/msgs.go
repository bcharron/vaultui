package tui

import (
	"github.com/bcharron/vaultui/internal/vault"
)

// kvEntriesLoadedMsg is shared by the engine detail Browse tab and the
// KV browser pages; each instance filters by mount+sub.
type kvEntriesLoadedMsg struct {
	mount, sub string
	entries    []vault.KVEntry
	err        error
}

// rolesLoadedMsg is shared by the auth detail and engine detail pages;
// each instance filters by mount path and role endpoint.
type rolesLoadedMsg struct {
	path     string
	endpoint string
	roles    []string
	err      error
}
