package tui

import (
	"fmt"
	"sort"
	"strings"

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
	cur, off int
	loading  bool
	err      error
	w, h     int
}

func newKVVersionsPage(cl *vaultapi.Client, m vault.Mount, sub string) *kvVersionsPage {
	return &kvVersionsPage{cl: cl, m: m, sub: sub, loading: true}
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
		p.w, p.h = msg.w, msg.h
	case kvMetaMsg:
		if msg.mount != p.m.Path || msg.sub != p.sub {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.versions = parseVersions(msg.data)
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(p.versions)-1 {
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

func (p *kvVersionsPage) enter() (Page, tea.Cmd) {
	if p.cur >= len(p.versions) {
		return p, nil
	}
	v := p.versions[p.cur]
	cl, m, sub := p.cl, p.m, p.sub
	return p, Push(newDetailPage(
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

func (p *kvVersionsPage) View() string {
	var b strings.Builder
	b.WriteString(dirStyle.Render(p.m.Path + p.sub))
	b.WriteString("\n")
	if p.loading {
		b.WriteString(loadingStyle.Render("loading versions ..."))
		return b.String()
	}
	if p.err != nil {
		b.WriteString(errStyle.Render("error: " + p.err.Error()))
		return b.String()
	}
	if len(p.versions) == 0 {
		b.WriteString(dimStyle.Render("no versions found"))
		return b.String()
	}
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(p.versions), rows)
	for i := p.off; i < min(p.off+rows, len(p.versions)); i++ {
		v := p.versions[i]
		flags := ""
		if v.current {
			flags += " " + typeStyle.Render("current")
		}
		if v.deleted {
			flags += " " + errStyle.Render("deleted")
		}
		if v.destroyed {
			flags += " " + errStyle.Render("destroyed")
		}
		created := truncate(v.created, 19)
		line := fmt.Sprintf("v%-4d %s", v.num, created)
		marker := "  "
		style := itemStyle
		if i == p.cur {
			marker = "> "
			style = selStyle
		}
		b.WriteString(style.Render(marker+line) + flags + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *kvVersionsPage) Header() string {
	return fmt.Sprintf("versions %s%s", p.m.Path, p.sub)
}

func (p *kvVersionsPage) Hints() string {
	return "up/down select  enter view version  r refresh  esc back  q quit"
}

func (p *kvVersionsPage) Typing() bool { return false }
