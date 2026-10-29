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
	filt    filter
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
			p.cur = pageJump(p.cur, max(p.h-3, 1), len(vis), false)
		case "pgdown":
			p.cur = pageJump(p.cur, max(p.h-3, 1), len(vis), true)
		case "enter":
			if p.cur < len(vis) {
				return p, Push(newEngineDetailPage(p.cl, vis[p.cur]))
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
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}

	rows := p.h - 2
	if fl := p.filt.view(); fl != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-20s %-16s %s", "PATH", "TYPE", "DESCRIPTION")) + listInfo(p.cur, len(vis), rows, "mounts") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")

	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		m := vis[i]
		typeDesc := m.Type
		if m.KVVersion > 0 {
			typeDesc = fmt.Sprintf("%s (v%d)", m.Type, m.KVVersion)
		}
		path := truncate(m.Path, 20)
		typ := truncate(typeDesc, 16)
		desc := truncate(m.Description, max(p.w-40, 0))

		if i == p.cur {
			row := fmt.Sprintf("  %-20s %-16s %s", path, typ, desc)
			b.WriteString(selBar(p.w, row) + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %s %s\n", p.filt.highlight(fmt.Sprintf("%-20s", path), itemStyle), p.filt.highlight(fmt.Sprintf("%-16s", typ), typeStyle), dimStyle.Render(desc)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// visible returns the mounts matching the filter.
func (p *enginesPage) visible() []vault.Mount {
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

func (p *enginesPage) Header() string { return "engines" }

func (p *enginesPage) Typing() bool { return p.filt.typing() }

func (p *enginesPage) Loading() bool { return p.loading }

func (p *enginesPage) Hints() string {
	return "up/down select  enter open engine  / filter  r refresh  esc back  q quit"
}

// window(cur, off, n, rows) scrolls a fixed-size window over n items
// keeping cur visible.
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

// pageJump moves cur by one page of rows, clamped to [0, n-1]; the
// pgup/pgdown navigation of the list pages.
func pageJump(cur, rows, n int, down bool) int {
	if rows < 1 {
		rows = 1
	}
	if n < 1 {
		return 0
	}
	if down {
		return min(cur+rows, n-1)
	}
	return max(cur-rows, 0)
}
