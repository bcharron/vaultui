package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type enginesPage struct {
	cl      *vaultapi.Client
	mounts  []vault.Mount
	cur     int
	off     int
	loading bool
	err     error
	w, h    int
}

type mountsLoadedMsg struct {
	mounts []vault.Mount
	err    error
}

func newEnginesPage(cl *vaultapi.Client) *enginesPage {
	return &enginesPage{cl: cl, loading: true}
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
		p.w, p.h = msg.w, msg.h
	case mountsLoadedMsg:
		p.loading = false
		p.err = msg.err
		p.mounts = msg.mounts
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(p.mounts)-1 {
				p.cur++
			}
		case "enter":
			if p.cur < len(p.mounts) {
				return p, Push(newEngineDetailPage(p.cl, p.mounts[p.cur]))
			}
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

func (p *enginesPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading mounts ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	if len(p.mounts) == 0 {
		return dimStyle.Render("no secret engines mounted")
	}

	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(p.mounts), rows)

	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-20s %-16s %s", "PATH", "TYPE", "DESCRIPTION")) + "\n\n")

	for i := p.off; i < min(p.off+rows, len(p.mounts)); i++ {
		m := p.mounts[i]
		typeDesc := m.Type
		if m.KVVersion > 0 {
			typeDesc = fmt.Sprintf("%s (v%d)", m.Type, m.KVVersion)
		}
		path := truncate(m.Path, 20)
		typ := truncate(typeDesc, 16)
		desc := truncate(m.Description, max(p.w-40, 0))

		marker := "  "
		pStyle, tStyle := itemStyle, dimStyle
		if i == p.cur {
			marker = "> "
			pStyle, tStyle = selStyle, typeStyle
		}
		b.WriteString(fmt.Sprintf("%s%s %s %s\n", marker, pStyle.Render(fmt.Sprintf("%-20s", path)), tStyle.Render(fmt.Sprintf("%-16s", typ)), dimStyle.Render(desc)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *enginesPage) Header() string { return "engines" }

func (p *enginesPage) Typing() bool { return false }

func (p *enginesPage) Hints() string {
	return "up/down select  enter open engine  r refresh  esc back  q quit"
}

func window(cur, off, n, rows int) (int, int) {
	if rows < 1 {
		rows = 1
	}
	if cur < off {
		off = cur
	}
	if cur >= off+rows {
		off = cur - rows + 1
	}
	if off > n-rows {
		off = n - rows
	}
	if off < 0 {
		off = 0
	}
	return cur, off
}
