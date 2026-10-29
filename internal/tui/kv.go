package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// kvRow is one row of the KV browser's list: the synthetic
// tune-config row on top, then the entries of the current directory.
type kvRow struct {
	config bool
	e      vault.KVEntry
}

// kvPage browses one directory level of a KV engine, with the engine's
// tune config as a synthetic first row.
type kvPage struct {
	cl      *vaultapi.Client
	m       vault.Mount
	sub     string // path under the mount, with trailing slash or ""
	entries []vault.KVEntry
	listPage[kvRow]
}

func newKVPage(cl *vaultapi.Client, m vault.Mount, sub string) *kvPage {
	p := &kvPage{cl: cl, m: m, sub: sub}
	p.loading = true
	p.items = func() []kvRow {
		rows := make([]kvRow, 0, len(p.entries))
		for _, e := range p.entries {
			rows = append(rows, kvRow{e: e})
		}
		return rows
	}
	p.visible = func() []kvRow {
		var out []kvRow
		if p.filt.match(engineTuneRowLabel) {
			out = append(out, kvRow{config: true})
		}
		if p.filt.empty() {
			return append(out, p.items()...)
		}
		for _, e := range p.entries {
			if p.filt.match(e.Name) {
				out = append(out, kvRow{e: e})
			}
		}
		return out
	}
	p.row = func(r kvRow, sel bool) string {
		if r.config {
			if sel {
				return selBar(p.w, "  "+engineTuneRowLabel)
			}
			return "  " + typeStyle.Render(engineTuneRowLabel)
		}
		if sel {
			return selBar(p.w, "  "+r.e.Name)
		}
		style := itemStyle
		if r.e.IsDir {
			style = dirStyle
		}
		return "  " + p.filt.highlight(r.e.Name, style)
	}
	p.onEnter = func(r kvRow) tea.Cmd {
		if r.config {
			cl, m := p.cl, p.m
			return Push(newDetailPage(
				fmt.Sprintf("config: %s", strings.TrimSuffix(m.Path, "/")),
				func() (string, []pageLink, error) {
					kvs, err := vault.Tune(cl, m.Path)
					if err != nil {
						return "", nil, err
					}
					return formatPairs(kvs), nil, nil
				},
			))
		}
		if r.e.IsDir {
			return Push(newKVPage(p.cl, p.m, p.sub+r.e.Name))
		}
		cl, m, sub, name := p.cl, p.m, p.sub, r.e.Name
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
		if p.cur < len(vis) && !vis[p.cur].config && !vis[p.cur].e.IsDir {
			return Push(newKVVersionsPage(p.cl, p.m, p.sub+vis[p.cur].e.Name)), true
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
