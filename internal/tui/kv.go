package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// kvEntriesLoadedMsg is shared by the engine detail Browse tab and the KV
// browser pages; each instance filters by mount+sub.
type kvEntriesLoadedMsg struct {
	mount, sub string
	entries    []vault.KVEntry
	err        error
}

// kvPage browses one directory level of a KV engine.
type kvPage struct {
	cl      *vaultapi.Client
	m       vault.Mount
	sub     string // path under the mount, with trailing slash or ""
	entries []vault.KVEntry
	filt    filter
	cur     int
	off     int
	loading bool
	err     error
	w, h    int
}

func newKVPage(cl *vaultapi.Client, m vault.Mount, sub string) *kvPage {
	return &kvPage{cl: cl, m: m, sub: sub, loading: true}
}

func (p *kvPage) Init() tea.Cmd {
	cl, m, sub := p.cl, p.m, p.sub
	return func() tea.Msg {
		entries, err := vault.ListKV(cl, m, sub)
		return kvEntriesLoadedMsg{mount: m.Path, sub: sub, entries: entries, err: err}
	}
}

func (p *kvPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case kvEntriesLoadedMsg:
		if msg.mount != p.m.Path || msg.sub != p.sub {
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
		case "v":
			if p.cur < len(vis) && !vis[p.cur].IsDir {
				return p, Push(newKVVersionsPage(p.cl, p.m, p.sub+vis[p.cur].Name))
			}
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

// visible returns the entries matching the filter.
func (p *kvPage) visible() []vault.KVEntry {
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

func (p *kvPage) enter() (Page, tea.Cmd) {
	vis := p.visible()
	if p.cur >= len(vis) {
		return p, nil
	}
	e := vis[p.cur]
	if e.IsDir {
		return p, Push(newKVPage(p.cl, p.m, p.sub+e.Name))
	}
	cl, m, sub, name := p.cl, p.m, p.sub, e.Name
	return p, Push(newDetailPage(
		"secret: "+m.Path+sub+name,
		func() (string, []pageLink, error) {
			data, meta, err := vault.ReadKV(cl, m, sub+name, 0)
			if err != nil {
				return "", nil, err
			}
			return secretDoc(data, meta), nil, nil
		},
	))
}

func (p *kvPage) View() string {
	path := p.m.Path + p.sub
	var b strings.Builder
	b.WriteString(dirStyle.Render(path))
	b.WriteString("\n")
	if p.loading {
		b.WriteString(loadingStyle.Render("loading keys ..."))
		return b.String()
	}
	if p.err != nil {
		b.WriteString(errStyle.Render("error listing keys: " + p.err.Error()))
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
	b.WriteString(listInfo(p.cur, len(vis), rows, "keys") + "\n")
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

func (p *kvPage) Header() string {
	return fmt.Sprintf("kv %s%s", p.m.Path, p.sub)
}

func (p *kvPage) Typing() bool { return p.filt.typing() }

func (p *kvPage) Loading() bool { return p.loading }

func (p *kvPage) Hints() string {
	return "up/down select  enter open  v versions  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
