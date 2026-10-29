package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// kvPage browses one directory level of a KV engine. The engine page's
// Keys row opens it at the mount root.
type kvPage struct {
	cl      *vaultapi.Client
	m       vault.Mount
	sub     string // path under the mount, with trailing slash or ""
	entries []vault.KVEntry
	listPage[vault.KVEntry]
}

func newKVPage(cl *vaultapi.Client, m vault.Mount, sub string) *kvPage {
	p := &kvPage{cl: cl, m: m, sub: sub}
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
			return Push(newKVPage(p.cl, p.m, p.sub+e.Name))
		}
		cl, m, sub, name := p.cl, p.m, p.sub, e.Name
		return Push(newDetailPage(
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
	// "v" opens the selected secret's version history.
	p.onKey = func(key string) (tea.Cmd, bool) {
		if key != "v" {
			return nil, false
		}
		vis := p.visible()
		if p.cur < len(vis) && !vis[p.cur].IsDir {
			return Push(newKVVersionsPage(p.cl, p.m, p.sub+vis[p.cur].Name)), true
		}
		return nil, false
	}
	p.reload = p.Init
	p.top = func() string { return dirStyle.Render(p.m.Path + p.sub) }
	p.noun = "keys"
	p.headLines = 3
	p.loadingText = "loading keys ..."
	p.emptyText = "(empty)"
	p.errPrefix = "error listing keys: "
	return p
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
		p.resize(msg.w, msg.h)
	case kvEntriesLoadedMsg:
		if msg.mount != p.m.Path || msg.sub != p.sub {
			return p, nil
		}
		p.entries = msg.entries
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *kvPage) Header() string {
	return fmt.Sprintf("kv %s%s", p.m.Path, p.sub)
}

func (p *kvPage) Hints() string {
	return "up/down select  enter open  v versions  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
