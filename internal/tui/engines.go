package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// enginesPage lists the mounted secret engines.
type enginesPage struct {
	cl     *vaultapi.Client
	mounts []vault.Mount
	listPage[vault.Mount]
}

type mountsLoadedMsg struct {
	mounts []vault.Mount
	err    error
}

func newEnginesPage(cl *vaultapi.Client) *enginesPage {
	p := &enginesPage{cl: cl}
	p.loading = true
	p.items = func() []vault.Mount { return p.mounts }
	p.visible = func() []vault.Mount {
		if p.filt.empty() {
			return p.mounts
		}
		var out []vault.Mount
		for _, m := range p.mounts {
			if p.filt.match(m.Path) || p.filt.match(m.Type) {
				out = append(out, m)
			}
		}
		return out
	}
	p.row = func(m vault.Mount, sel bool) string {
		typeDesc := m.Type
		if m.KVVersion > 0 {
			typeDesc = fmt.Sprintf("%s (v%d)", m.Type, m.KVVersion)
		}
		path := truncate(m.Path, 20)
		typ := truncate(typeDesc, 16)
		desc := truncate(m.Description, max(p.w-40, 0))
		if sel {
			return selBar(p.w, fmt.Sprintf("  %-20s %-16s %s", path, typ, desc))
		}
		return fmt.Sprintf("  %s %s %s",
			p.filt.highlight(fmt.Sprintf("%-20s", path), itemStyle),
			p.filt.highlight(fmt.Sprintf("%-16s", typ), typeStyle),
			dimStyle.Render(desc))
	}
	p.onEnter = func(m vault.Mount) tea.Cmd {
		return Push(newEngineDetailPage(p.cl, m))
	}
	p.reload = p.Init
	p.title = dimStyle.Render(fmt.Sprintf("  %-20s %-16s %s", "PATH", "TYPE", "DESCRIPTION"))
	p.noun = "mounts"
	p.headLines = 2
	p.loadingText = "loading mounts ..."
	p.emptyText = "no secret engines mounted"
	p.errPrefix = "error: "
	return p
}

func (p *enginesPage) Init() tea.Cmd {
	cl := p.cl
	return func() tea.Msg {
		m, err := vault.ListMounts(cl)
		return mountsLoadedMsg{mounts: m, err: err}
	}
}

func (p *enginesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case mountsLoadedMsg:
		p.mounts = msg.mounts
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *enginesPage) Header() string { return "engines" }

func (p *enginesPage) Hints() string {
	return "up/down select  enter open engine  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
