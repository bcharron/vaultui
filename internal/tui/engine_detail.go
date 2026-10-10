package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"vaultui/internal/vault"
)

type engineDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	tab  int

	// Config tab
	cfgText    string
	cfgLoading bool
	cfgErr     error
	vp         viewport.Model

	// Roles tab
	rolesLoading bool
	rolesErr     error
	roles        []string
	rcur, roff   int

	// Browse tab (KV engines)
	entLoading bool
	entErr     error
	entries    []vault.KVEntry
	ecur, eoff int
}

type tuneLoadedMsg struct {
	path string
	kv   []vault.KV
	err  error
}

type rolesLoadedMsg struct {
	path  string
	roles []string
	err   error
}

func newEngineDetailPage(cl *vaultapi.Client, m vault.Mount) *engineDetailPage {
	return &engineDetailPage{
		cl:           cl,
		m:            m,
		cfgLoading:   true,
		rolesLoading: m.Type != "kv",
		entLoading:   m.Type == "kv",
		vp:           viewport.New(0, 0),
	}
}

func (p *engineDetailPage) Init() tea.Cmd {
	cl, m := p.cl, p.m
	cmds := []tea.Cmd{
		func() tea.Msg {
			kv, err := vault.Tune(cl, m.Path)
			return tuneLoadedMsg{path: m.Path, kv: kv, err: err}
		},
	}
	if m.Type != "kv" {
		cmds = append(cmds, func() tea.Msg {
			roles, err := vault.ListRoles(cl, m.Path, m.Type)
			return rolesLoadedMsg{path: m.Path, roles: roles, err: err}
		})
	}
	if m.Type == "kv" {
		cmds = append(cmds, func() tea.Msg {
			entries, err := vault.ListKV(cl, m, "")
			return kvEntriesLoadedMsg{mount: m.Path, sub: "", entries: entries, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (p *engineDetailPage) tabs() []string {
	if p.m.Type == "kv" {
		return []string{"Config", "Browse"}
	}
	if p.m.Type == "transit" {
		return []string{"Config", "Keys"}
	}
	return []string{"Config", "Roles"}
}

func (p *engineDetailPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
		p.layout()
	case tuneLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		p.cfgLoading = false
		p.cfgErr = msg.err
		if msg.err == nil {
			p.cfgText = formatPairs(msg.kv)
		}
		p.layout()
	case rolesLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		p.rolesLoading = false
		p.rolesErr = msg.err
		p.roles = msg.roles
	case kvEntriesLoadedMsg:
		if msg.mount != p.m.Path || msg.sub != "" {
			return p, nil
		}
		p.entLoading = false
		p.entErr = msg.err
		p.entries = msg.entries
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *engineDetailPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
	tabs := p.tabs()
	switch msg.String() {
	case "tab", "right", "l":
		if p.tab < len(tabs)-1 {
			p.tab++
		}
		return p, nil
	case "shift+tab", "left", "h":
		if p.tab > 0 {
			p.tab--
		}
		return p, nil
	case "r":
		np := newEngineDetailPage(p.cl, p.m)
		np.w, np.h = p.w, p.h
		np.tab = p.tab
		np.layout()
		return np, np.Init()
	case "up", "k", "ctrl+k":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
			return p, nil
		}
		if p.isKV() {
			if p.ecur > 0 {
				p.ecur--
			}
		} else if p.rcur > 0 {
			p.rcur--
		}
		return p, nil
	case "down", "j":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
			return p, nil
		}
		if p.isKV() {
			if p.ecur < len(p.entries)-1 {
				p.ecur++
			}
		} else if p.rcur < len(p.roles)-1 {
			p.rcur++
		}
		return p, nil
	case "pgup", "pgdown", "home", "end", "g", "G":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
		}
		return p, nil
	case "enter":
		return p.enter()
	}
	return p, nil
}

func (p *engineDetailPage) isKV() bool { return p.m.Type == "kv" }

func (p *engineDetailPage) enter() (Page, tea.Cmd) {
	cl, m := p.cl, p.m
	if p.tab == 0 {
		return p, nil
	}
	if p.isKV() {
		if p.ecur >= len(p.entries) {
			return p, nil
		}
		e := p.entries[p.ecur]
		if e.IsDir {
			return p, Push(newKVPage(cl, m, e.Name))
		}
		return p, Push(newDetailPage(
			"secret: "+m.Path+e.Name,
			func() (string, error) {
				data, meta, err := vault.ReadKV(cl, m, e.Name)
				if err != nil {
					return "", err
				}
				return secretDoc(data, meta), nil
			},
		))
	}
	// Roles / Keys tab
	if p.rcur >= len(p.roles) {
		return p, nil
	}
	name := p.roles[p.rcur]
	return p, Push(newDetailPage(
		fmt.Sprintf("%s: %s", p.tabs()[1], name),
		func() (string, error) {
			data, err := vault.ReadRole(cl, m.Path, m.Type, name)
			if err != nil {
				return "", err
			}
			return formatMap(data), nil
		},
	))
}

// layout recomputes viewport geometry and re-wraps its content.
func (p *engineDetailPage) layout() {
	p.vp.Width = p.w
	p.vp.Height = p.h - 1 // below the tab bar
	p.vp.SetContent(wrapText(p.cfgText, p.w))
}

func (p *engineDetailPage) View() string {
	var b strings.Builder
	b.WriteString(p.tabBar())
	b.WriteString("\n")
	switch {
	case p.tab == 0:
		b.WriteString(p.viewConfig())
	case p.isKV():
		b.WriteString(p.viewEntries())
	default:
		b.WriteString(p.viewRoles())
	}
	return b.String()
}

func (p *engineDetailPage) tabBar() string {
	parts := make([]string, 0, 2)
	for i, t := range p.tabs() {
		if i == p.tab {
			parts = append(parts, activeTabStyle.Render("["+t+"]"))
		} else {
			parts = append(parts, dimStyle.Render(" "+t+" "))
		}
	}
	info := fmt.Sprintf("%s %s", p.m.Type, p.m.Accessor)
	return strings.Join(parts, " ") + "   " + dimStyle.Render(info)
}

func (p *engineDetailPage) viewConfig() string {
	if p.cfgLoading {
		return loadingStyle.Render("loading config ...")
	}
	if p.cfgErr != nil {
		return errStyle.Render("error reading sys/mounts/" + p.m.Path + "tune: " + p.cfgErr.Error())
	}
	if p.cfgText == "" {
		return dimStyle.Render("no tune data returned")
	}
	return p.vp.View()
}

func (p *engineDetailPage) viewRoles() string {
	if p.rolesLoading {
		return loadingStyle.Render("loading roles ...")
	}
	if p.rolesErr != nil {
		return errStyle.Render("error listing roles: " + p.rolesErr.Error())
	}
	if len(p.roles) == 0 {
		return dimStyle.Render("no roles found")
	}
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.rcur, p.roff = window(p.rcur, p.roff, len(p.roles), rows)
	var b strings.Builder
	for i := p.roff; i < min(p.roff+rows, len(p.roles)); i++ {
		if i == p.rcur {
			b.WriteString(selStyle.Render("> "+p.roles[i]) + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+p.roles[i]) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *engineDetailPage) viewEntries() string {
	if p.entLoading {
		return loadingStyle.Render("loading entries ...")
	}
	if p.entErr != nil {
		return errStyle.Render("error listing keys: " + p.entErr.Error())
	}
	if len(p.entries) == 0 {
		return dimStyle.Render("no keys at this path (or no list permission)")
	}
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.ecur, p.eoff = window(p.ecur, p.eoff, len(p.entries), rows)
	var b strings.Builder
	for i := p.eoff; i < min(p.eoff+rows, len(p.entries)); i++ {
		e := p.entries[i]
		marker := "  "
		style := itemStyle
		if e.IsDir {
			style = dirStyle
		}
		if i == p.ecur {
			marker = "> "
			style = selStyle
		}
		b.WriteString(style.Render(marker+e.Name) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *engineDetailPage) Header() string {
	return fmt.Sprintf("engines / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *engineDetailPage) Hints() string {
	return "left/right tabs  up/down select  enter open  r refresh  esc back  q quit"
}
