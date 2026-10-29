package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type leasesLoadedMsg struct {
	prefix  string
	entries []vault.KVEntry
	err     error
}

// leasesPage browses the sys/leases/lookup tree: prefixes drill down,
// leaves open the lease's lookup document.
type leasesPage struct {
	cl      *vaultapi.Client
	prefix  string
	entries []vault.KVEntry
	listPage[vault.KVEntry]
}

func newLeasesPage(cl *vaultapi.Client, prefix string) *leasesPage {
	p := &leasesPage{cl: cl, prefix: prefix}
	p.loading = true
	p.items = func() []vault.KVEntry { return p.entries }
	p.visible = func() []vault.KVEntry {
		if p.filt.empty() {
			return p.entries
		}
		var out []vault.KVEntry
		for _, e := range p.entries {
			if p.filt.match(e.Name) {
				out = append(out, e)
			}
		}
		return out
	}
	p.row = func(e vault.KVEntry, sel bool) string {
		if sel {
			return selBar(p.w, "  "+e.Name)
		}
		style := itemStyle
		if e.IsDir {
			style = dirStyle
		}
		return "  " + p.filt.highlight(e.Name, style)
	}
	p.onEnter = func(e vault.KVEntry) tea.Cmd {
		if e.IsDir {
			return Push(newLeasesPage(p.cl, p.prefix+e.Name))
		}
		leaseID := p.prefix + e.Name
		cl := p.cl
		return Push(newDetailPage(
			"lease: "+leaseID,
			func() (string, []pageLink, error) {
				data, err := vault.LookupLease(cl, leaseID)
				if err != nil {
					return "", nil, err
				}
				return formatMap(data), nil, nil
			},
		))
	}
	p.reload = p.Init
	p.top = func() string { return dirStyle.Render("/" + p.prefix) }
	p.noun = "leases"
	p.headLines = 3
	p.loadingText = "loading leases ..."
	p.emptyText = "(empty)"
	p.errPrefix = "error listing leases: "
	return p
}

func (p *leasesPage) Init() tea.Cmd {
	cl, prefix := p.cl, p.prefix
	return func() tea.Msg {
		entries, err := vault.ListLeases(cl, prefix)
		return leasesLoadedMsg{prefix: prefix, entries: entries, err: err}
	}
}

func (p *leasesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case leasesLoadedMsg:
		if msg.prefix != p.prefix {
			return p, nil
		}
		p.entries = msg.entries
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *leasesPage) Header() string {
	return fmt.Sprintf("leases /%s", p.prefix)
}

func (p *leasesPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
