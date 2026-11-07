package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// aliasListPage lists the identity group aliases assigned to one auth
// method's accessor, each row showing the group entity the alias points
// to. Enter on a row jumps to that group's identity page.
type aliasListPage struct {
	cl        *vaultapi.Client
	mountPath string // the auth mount path, e.g. "jwt/"
	accessor  string

	aliases    []vault.GroupAlias
	groupNames map[string]string // canonical_id -> group name

	listPage[vault.GroupAlias]
}

func newAliasListPage(cl *vaultapi.Client, mountPath, accessor string) *aliasListPage {
	p := &aliasListPage{cl: cl, mountPath: mountPath, accessor: accessor, groupNames: map[string]string{}}
	p.loading = true
	p.items = func() []vault.GroupAlias { return p.aliases }
	p.visible = func() []vault.GroupAlias {
		if p.filt.empty() {
			return p.aliases
		}
		var out []vault.GroupAlias
		for _, a := range p.aliases {
			if p.filt.match(a.Name) || p.filt.match(p.groupDisplay(a)) {
				out = append(out, a)
			}
		}
		return out
	}
	p.row = func(a vault.GroupAlias, sel bool) string {
		name := truncate(a.Name, 30)
		grp := p.groupDisplay(a)
		if sel {
			return selBar(p.w, fmt.Sprintf("  %-30s %s", name, grp))
		}
		return fmt.Sprintf("  %s %s",
			p.filt.highlight(fmt.Sprintf("%-30s", name), itemStyle),
			dimStyle.Render("-> "+grp))
	}
	p.onEnter = func(a vault.GroupAlias) tea.Cmd {
		id := aliasCanonicalID(a)
		return Push(groupDetailByID(p.cl, id, p.groupNames[id]))
	}
	p.reload = p.Init
	p.title = dimStyle.Render(fmt.Sprintf("  %-30s %s", "ALIAS", "GROUP"))
	p.noun = "aliases"
	p.headLines = 2
	p.loadingText = "loading group aliases ..."
	p.emptyText = "no group aliases on this auth method"
	p.errPrefix = "error: "
	return p
}

func (p *aliasListPage) Init() tea.Cmd {
	cl, acc := p.cl, p.accessor
	return func() tea.Msg {
		aliases, err := vault.ListGroupAliasesByAccessor(cl, acc)
		if err != nil {
			return aliasesLoadedMsg{accessor: acc, err: err}
		}
		// Resolve each alias's group name so rows can show the
		// corresponding group entity; a group that cannot be read
		// falls back to its id in the row.
		names := map[string]string{}
		for _, a := range aliases {
			id := aliasCanonicalID(a)
			if id == "" {
				continue
			}
			if g, gerr := vault.ReadGroupByID(cl, id); gerr == nil {
				if n, _ := g["name"].(string); n != "" {
					names[id] = n
				}
			}
		}
		return aliasesLoadedMsg{accessor: acc, aliases: aliases, groupNames: names}
	}
}

func (p *aliasListPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case aliasesLoadedMsg:
		if msg.accessor != p.accessor {
			return p, nil
		}
		p.aliases = msg.aliases
		if msg.groupNames != nil {
			p.groupNames = msg.groupNames
		}
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *aliasListPage) Header() string {
	return fmt.Sprintf("auth / %s / group aliases", strings.TrimSuffix(p.mountPath, "/"))
}

func (p *aliasListPage) Hints() string {
	return "up/down select  enter open group  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

// groupDisplay is the group an alias points to: its name once
// resolved, else the truncated canonical id.
func (p *aliasListPage) groupDisplay(a vault.GroupAlias) string {
	id := aliasCanonicalID(a)
	if n, ok := p.groupNames[id]; ok && n != "" {
		return n
	}
	if id == "" {
		return "(no group)"
	}
	return truncate(id, 12)
}

// aliasCanonicalID is the id of the group entity an alias maps to.
func aliasCanonicalID(a vault.GroupAlias) string {
	id, _ := a.Data["canonical_id"].(string)
	return id
}
