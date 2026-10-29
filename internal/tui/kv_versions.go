package tui

import (
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// kvVersion is one entry of a KV v2 secret's version history.
type kvVersion struct {
	num       int
	created   string
	deleted   bool
	destroyed bool
	current   bool
}

type kvMetaMsg struct {
	mount, sub string
	data       map[string]interface{}
	err        error
}

// kvVersionsPage lists a KV v2 secret's versions; enter opens that
// version's data.
type kvVersionsPage struct {
	cl       *vaultapi.Client
	m        vault.Mount
	sub      string
	versions []kvVersion
	listPage[kvVersion]
}

func newKVVersionsPage(cl *vaultapi.Client, m vault.Mount, sub string) *kvVersionsPage {
	p := &kvVersionsPage{cl: cl, m: m, sub: sub}
	p.loading = true
	p.items = func() []kvVersion { return p.versions }
	p.visible = func() []kvVersion {
		if p.filt.empty() {
			return p.versions
		}
		var out []kvVersion
		for _, v := range p.versions {
			if p.filt.match(fmt.Sprintf("v%d", v.num)) || p.filt.match(v.created) {
				out = append(out, v)
			}
		}
		return out
	}
	p.row = func(v kvVersion, sel bool) string {
		line := fmt.Sprintf("v%-4d %s", v.num, truncate(v.created, 19))
		if sel {
			row := "  " + line
			if v.current {
				row += " current"
			}
			if v.deleted {
				row += " deleted"
			}
			if v.destroyed {
				row += " destroyed"
			}
			return selBar(p.w, row)
		}
		var flags string
		if v.current {
			flags += " " + typeStyle.Render("current")
		}
		if v.deleted {
			flags += " " + errStyle.Render("deleted")
		}
		if v.destroyed {
			flags += " " + errStyle.Render("destroyed")
		}
		return itemStyle.Render("  "+line) + flags
	}
	p.onEnter = func(v kvVersion) tea.Cmd {
		cl, m, sub := p.cl, p.m, p.sub
		return Push(newDetailPage(
			fmt.Sprintf("secret: %s%s (v%d)", m.Path, sub, v.num),
			func() (string, []pageLink, error) {
				data, meta, err := vault.ReadKV(cl, m, sub, v.num)
				if err != nil {
					return "", nil, err
				}
				return secretDoc(data, meta), nil, nil
			},
		))
	}
	p.reload = p.Init
	p.top = func() string { return dirStyle.Render(p.m.Path + p.sub) }
	p.noun = "versions"
	p.headLines = 3
	p.loadingText = "loading versions ..."
	p.emptyText = "no versions found"
	p.errPrefix = "error: "
	return p
}

func (p *kvVersionsPage) Init() tea.Cmd {
	cl, m, sub := p.cl, p.m, p.sub
	return func() tea.Msg {
		data, err := vault.KVMetadata(cl, m, sub)
		return kvMetaMsg{mount: m.Path, sub: sub, data: data, err: err}
	}
}

func (p *kvVersionsPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case kvMetaMsg:
		if msg.mount != p.m.Path || msg.sub != p.sub {
			return p, nil
		}
		p.versions = parseVersions(msg.data)
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func parseVersions(meta map[string]interface{}) []kvVersion {
	if meta == nil {
		return nil
	}
	versionsRaw, _ := meta["versions"].(map[string]interface{})
	current := 0
	if f, ok := meta["current_version"].(float64); ok {
		current = int(f)
	}
	out := make([]kvVersion, 0, len(versionsRaw))
	for numStr, vRaw := range versionsRaw {
		var num int
		if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
			continue
		}
		v, _ := vRaw.(map[string]interface{})
		kv := kvVersion{num: num, current: num == current}
		if s, ok := v["created_time"].(string); ok {
			kv.created = s
		}
		if s, ok := v["deletion_time"].(string); ok && s != "" {
			kv.deleted = true
		}
		if b, ok := v["destroyed"].(bool); ok {
			kv.destroyed = b
		}
		out = append(out, kv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].num > out[j].num })
	return out
}

func (p *kvVersionsPage) Header() string {
	return fmt.Sprintf("versions %s%s", p.m.Path, p.sub)
}

func (p *kvVersionsPage) Hints() string {
	return "up/down select  enter view version  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
