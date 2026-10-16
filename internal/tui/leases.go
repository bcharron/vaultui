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
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(p.entries)-1 {
				p.cur++
			}
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

func (p *leasesPage) enter() (Page, tea.Cmd) {
	if p.cur >= len(p.entries) {
		return p, nil
	}
	e := p.entries[p.cur]
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
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(p.entries), rows)
	for i := p.off; i < min(p.off+rows, len(p.entries)); i++ {
		e := p.entries[i]
		style := itemStyle
		if e.IsDir {
			style = dirStyle
		}
		if i == p.cur {
			style = selStyle
		}
		marker := "  "
		if i == p.cur {
			marker = "> "
		}
		b.WriteString(style.Render(marker+e.Name) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *leasesPage) Header() string {
	return fmt.Sprintf("leases /%s", p.prefix)
}

func (p *leasesPage) Hints() string {
	return "up/down select  enter open  r refresh  esc back  q quit"
}

func (p *leasesPage) Typing() bool { return false }
