package tui

import (
	"fmt"
	"strings"

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
	filt    filter
	cur     int
	off     int
	loading bool
	err     error
	w, h    int
}

func newLeasesPage(cl *vaultapi.Client, prefix string) *leasesPage {
	return &leasesPage{cl: cl, prefix: prefix, loading: true}
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
		p.w, p.h = msg.w, msg.h
	case leasesLoadedMsg:
		if msg.prefix != p.prefix {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.entries = msg.entries
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		if p.filt.typing() {
			if p.filt.update(msg) {
				p.cur, p.off = 0, 0
				return p, nil
			}
		} else if msg.String() == "/" {
			p.filt.active = true
			return p, nil
		}
		vis := p.visible()
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(vis)-1 {
				p.cur++
			}
		case "pgup":
			p.cur = pageJump(p.cur, max(p.h-4, 1), len(vis), false)
		case "pgdown":
			p.cur = pageJump(p.cur, max(p.h-4, 1), len(vis), true)
		case "enter":
			return p.enter()
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

// visible returns the entries matching the filter.
func (p *leasesPage) visible() []vault.KVEntry {
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

func (p *leasesPage) enter() (Page, tea.Cmd) {
	vis := p.visible()
	if p.cur >= len(vis) {
		return p, nil
	}
	e := vis[p.cur]
	if e.IsDir {
		return p, Push(newLeasesPage(p.cl, p.prefix+e.Name))
	}
	leaseID := p.prefix + e.Name
	cl := p.cl
	return p, Push(newDetailPage(
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

func (p *leasesPage) View() string {
	var b strings.Builder
	b.WriteString(dirStyle.Render("/" + p.prefix))
	b.WriteString("\n")
	if p.loading {
		b.WriteString(loadingStyle.Render("loading leases ..."))
		return b.String()
	}
	if p.err != nil {
		b.WriteString(errStyle.Render("error listing leases: " + p.err.Error()))
		return b.String()
	}
	if len(p.entries) == 0 {
		b.WriteString(dimStyle.Render("(empty)"))
		return b.String()
	}
	vis := p.visible()
	if len(vis) == 0 {
		b.WriteString(p.filt.view() + "\n\n")
		b.WriteString(dimStyle.Render("(no matches)"))
		return b.String()
	}
	rows := p.h - 3
	if fl := p.filt.view(); fl != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)
	b.WriteString(listInfo(p.cur, len(vis), rows, "leases") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		e := vis[i]
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+e.Name) + "\n")
			continue
		}
		style := itemStyle
		if e.IsDir {
			style = dirStyle
		}
		b.WriteString("  " + p.filt.highlight(e.Name, style) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *leasesPage) Header() string {
	return fmt.Sprintf("leases /%s", p.prefix)
}

func (p *leasesPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  q quit"
}

func (p *leasesPage) Typing() bool { return p.filt.typing() }

func (p *leasesPage) Loading() bool { return p.loading }
