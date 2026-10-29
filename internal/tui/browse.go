package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// roleCfg is one role-like item's configuration (an auth role, a
// transit key, an AWS role, ...), fetched lazily for the expansion
// pane of the auth and engine pages.
type roleCfg struct {
	loading bool
	err     error
	data    map[string]interface{}
}

// roleCfgMsg delivers one role-like item's configuration. key is the
// group endpoint plus the item name: the expansion cache's key.
type roleCfgMsg struct {
	path string // full mount path prefix; guards stale messages
	key  string // endpoint + "/" + name
	data map[string]interface{}
	err  error
}

// roleCfgFetch reads one role-like item's configuration. mountPath is
// the full API prefix of the mount ("transit/", "auth/jwt/").
func roleCfgFetch(cl *vaultapi.Client, mountPath, endpoint, name string) tea.Cmd {
	return func() tea.Msg {
		data, err := vault.ReadRole(cl, mountPath, endpoint, name)
		return roleCfgMsg{path: mountPath, key: endpoint + "/" + name, data: data, err: err}
	}
}

// truncateLines caps the expansion pane at area lines, replacing the
// overflow with a dim indicator: rich documents elide rather than
// push the layout around.
func truncateLines(lines []string, area int) []string {
	if area < 1 {
		area = 1
	}
	if len(lines) > area {
		return append(lines[:area-1:area-1], dimStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-area+1)))
	}
	return lines
}

// cfgExpansionLines renders a role-like item's summary for the
// expansion pane: the policies it grants, then its scalar fields.
// The full document stays one enter away.
func cfgExpansionLines(name string, cfg *roleCfg) []string {
	lines := []string{titleStyle.Render(name)}
	if cfg == nil {
		return lines
	}
	if cfg.loading {
		return append(lines, dimStyle.Render("loading ..."))
	}
	if cfg.err != nil {
		return append(lines, dimStyle.Render("error: "+cfg.err.Error()))
	}
	if pols := extractPolicies(cfg.data); len(pols) > 0 {
		lines = append(lines, keyStyle.Render("policies: ")+valStyle.Render(strings.Join(pols, ", ")))
	}
	keys := make([]string, 0, len(cfg.data))
	for k := range cfg.data {
		if k == "policies" || k == "token_policies" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, keyStyle.Render(k+": ")+cfgValue(cfg.data[k]))
	}
	return lines
}

// cfgValue renders one config value for the expansion pane: scalars
// and plain lists inline; nested structures collapse to a placeholder
// (the full document shows them properly).
func cfgValue(v interface{}) string {
	switch tv := v.(type) {
	case map[string]interface{}:
		return dimStyle.Render("(object)")
	case []interface{}:
		if hasMapElem(tv) {
			return dimStyle.Render("(list)")
		}
		return styledValue(tv)
	default:
		return styledValue(v)
	}
}
