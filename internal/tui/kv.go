package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"vaultui/internal/vault"
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

func (p *kvPage) enter() (Page, tea.Cmd) {
	if p.cur >= len(p.entries) {
		return p, nil
	}
	e := p.entries[p.cur]
	if e.IsDir {
		return p, Push(newKVPage(p.cl, p.m, p.sub+e.Name))
	}
	cl, m, sub, name := p.cl, p.m, p.sub, e.Name
	return p, Push(newDetailPage(
		"secret: "+m.Path+sub+name,
		func() (string, []pageLink, error) {
			data, meta, err := vault.ReadKV(cl, m, sub+name)
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

func (p *kvPage) Header() string {
	return fmt.Sprintf("kv %s%s", p.m.Path, p.sub)
}

func (p *kvPage) Hints() string {
	return "up/down select  enter open  r refresh  esc back  q quit"
}
