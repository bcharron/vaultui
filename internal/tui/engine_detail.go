package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type engineDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	tab  int
	filt filter

	// Config tab (sys/mounts/{path}/tune)
	cfgText    string
	cfgLoading bool
	cfgErr     error
	vp         viewport.Model

	// Engine Config tab (aws config/root+lease, gcp config): shown only
	// when the engine type has dedicated config endpoints.
	ecfgEndpoints []string
	ecfgLoading   bool
	ecfgText      string
	evp           viewport.Model

	// Role group tabs (Roles, Keys, Rolesets, ...): one per group.
	groups []roleGroupState

	// Browse tab (KV engines)
	entLoading bool
	entErr     error
	entries    []vault.KVEntry
	ecur, eoff int
}

// roleGroupState is one role-like tab: a list of names with cursors and
// optional inline annotations (e.g. the AWS credential type).
type roleGroupState struct {
	label    string
	endpoint string
	loading  bool
	err      error
	roles    []string
	cur, off int
	ann      map[string]string
}

type tuneLoadedMsg struct {
	path string
	kv   []vault.KV
	err  error
}

type roleAnnMsg struct {
	path     string
	endpoint string
	role     string
	ann      string
}

type engineCfgLoadedMsg struct {
	path string
	text string
}

func newEngineDetailPage(cl *vaultapi.Client, m vault.Mount) *engineDetailPage {
	p := &engineDetailPage{
		cl:            cl,
		m:             m,
		cfgLoading:    true,
		ecfgEndpoints: vault.EngineConfigEndpoints(m.Type),
		ecfgLoading:   len(vault.EngineConfigEndpoints(m.Type)) > 0,
		vp:            viewport.New(0, 0),
		evp:           viewport.New(0, 0),
		entLoading:    m.Type == "kv",
	}
	if m.Type != "kv" {
		for _, g := range vault.RoleGroups(m.Type) {
			p.groups = append(p.groups, roleGroupState{
				label:    g.Label,
				endpoint: g.Endpoint,
				loading:  true,
				ann:      map[string]string{},
			})
		}
	}
	return p
}

func (p *engineDetailPage) Init() tea.Cmd {
	cl, m := p.cl, p.m
	cmds := []tea.Cmd{
		func() tea.Msg {
			kv, err := vault.Tune(cl, m.Path)
			return tuneLoadedMsg{path: m.Path, kv: kv, err: err}
		},
	}
	for i := range p.groups {
		g := p.groups[i]
		cmds = append(cmds, func() tea.Msg {
			roles, err := vault.ListRoles(cl, m.Path, g.endpoint)
			return rolesLoadedMsg{path: m.Path, endpoint: g.endpoint, roles: roles, err: err}
		})
	}
	if p.ecfgLoading {
		eps := p.ecfgEndpoints
		cmds = append(cmds, func() tea.Msg {
			return engineCfgLoadedMsg{path: m.Path, text: engineConfigDoc(cl, m.Path, eps)}
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
	tabs := []string{"Config"}
	if p.hasEngineConfig() {
		tabs = append(tabs, "Engine Config")
	}
	for _, g := range p.groups {
		tabs = append(tabs, g.label)
	}
	return tabs
}

func (p *engineDetailPage) hasEngineConfig() bool { return len(p.ecfgEndpoints) > 0 }

// groupTab returns the role group state for the active tab, or nil when
// the active tab is not a role group.
func (p *engineDetailPage) groupTab() *roleGroupState {
	if p.m.Type == "kv" {
		return nil
	}
	i := p.tab - 1
	if p.hasEngineConfig() {
		i--
	}
	if i < 0 || i >= len(p.groups) {
		return nil
	}
	return &p.groups[i]
}

func (p *engineDetailPage) groupByEndpoint(endpoint string) *roleGroupState {
	for i := range p.groups {
		if p.groups[i].endpoint == endpoint {
			return &p.groups[i]
		}
	}
	return nil
}

// groupAnnotated reports whether roles in this group get an inline
// annotation: the AWS credential type, the GCP roleset secret type.
func (p *engineDetailPage) groupAnnotated(g *roleGroupState) bool {
	switch {
	case p.m.Type == "aws" && g.endpoint == "roles":
		return true
	case p.m.Type == "gcp" && g.endpoint == "roleset":
		return true
	}
	return false
}

// loadRoleAnns reads each listed role's config (capped) so the group tab
// can show its inline annotation.
func (p *engineDetailPage) loadRoleAnns(g *roleGroupState) tea.Cmd {
	if g.err != nil || !p.groupAnnotated(g) || len(g.roles) == 0 {
		return nil
	}
	cl, m := p.cl, p.m
	names := g.roles
	if len(names) > 50 {
		names = names[:50]
	}
	cmds := make([]tea.Cmd, 0, len(names))
	for _, name := range names {
		role, endpoint := name, g.endpoint
		cmds = append(cmds, func() tea.Msg {
			data, err := vault.ReadRole(cl, m.Path, endpoint, role)
			if err != nil {
				return roleAnnMsg{path: m.Path, endpoint: endpoint, role: role}
			}
			return roleAnnMsg{path: m.Path, endpoint: endpoint, role: role, ann: roleAnnotation(m.Type, data)}
		})
	}
	return tea.Batch(cmds...)
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
	case engineCfgLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		p.ecfgLoading = false
		p.ecfgText = msg.text
		p.layout()
	case rolesLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		g := p.groupByEndpoint(msg.endpoint)
		if g == nil {
			return p, nil
		}
		g.loading = false
		g.err = msg.err
		g.roles = msg.roles
		g.cur, g.off = 0, 0
		return p, p.loadRoleAnns(g)
	case roleAnnMsg:
		if msg.path != p.m.Path || msg.ann == "" {
			return p, nil
		}
		if g := p.groupByEndpoint(msg.endpoint); g != nil {
			g.ann[msg.role] = msg.ann
		}
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
	if p.filt.typing() {
		if p.filt.update(msg) {
			g := p.groupTab()
			if g != nil {
				g.cur, g.off = 0, 0
			}
			p.ecur, p.eoff = 0, 0
			return p, nil
		}
	} else if msg.String() == "/" && p.textTab() < 0 {
		p.filt.active = true
		return p, nil
	}
	g := p.groupTab()
	gvis := []string(nil)
	if g != nil {
		gvis = p.visibleGroupRoles(g)
	}
	evis := p.visibleEntries()
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
		switch p.textTab() {
		case 0:
			p.vp, _ = p.vp.Update(msg)
		case 1:
			p.evp, _ = p.evp.Update(msg)
		default:
			if p.isKV() {
				if p.ecur > 0 {
					p.ecur--
				}
			} else if g != nil && g.cur > 0 {
				g.cur--
			}
		}
		return p, nil
	case "down", "j":
		switch p.textTab() {
		case 0:
			p.vp, _ = p.vp.Update(msg)
		case 1:
			p.evp, _ = p.evp.Update(msg)
		default:
			if p.isKV() {
				if p.ecur < len(evis)-1 {
					p.ecur++
				}
			} else if g != nil && g.cur < len(gvis)-1 {
				g.cur++
			}
		}
		return p, nil
	case "pgup", "pgdown":
		switch p.textTab() {
		case 0:
			p.vp, _ = p.vp.Update(msg)
		case 1:
			p.evp, _ = p.evp.Update(msg)
		default:
			down := msg.String() == "pgdown"
			if p.isKV() {
				p.ecur = pageJump(p.ecur, p.listRows(), len(evis), down)
			} else if g != nil {
				g.cur = pageJump(g.cur, p.listRows(), len(gvis), down)
			}
		}
		return p, nil
	case "home", "end", "g", "G":
		switch p.textTab() {
		case 0:
			p.vp, _ = p.vp.Update(msg)
		case 1:
			p.evp, _ = p.evp.Update(msg)
		}
		return p, nil
	case "v":
		if p.isKV() && p.tab == 1 && p.ecur < len(evis) && !evis[p.ecur].IsDir {
			return p, Push(newKVVersionsPage(p.cl, p.m, evis[p.ecur].Name))
		}
		return p, nil
	case "enter":
		return p.enter()
	}
	return p, nil
}

// textTab reports which scrollable text tab is active: 0 for the tune
// Config tab, 1 for the Engine Config tab, -1 otherwise.
func (p *engineDetailPage) textTab() int {
	if p.tab == 0 {
		return 0
	}
	if p.hasEngineConfig() && p.tab == 1 {
		return 1
	}
	return -1
}

func (p *engineDetailPage) isKV() bool { return p.m.Type == "kv" }

// listRows is the number of list rows that fit under the tab bar.
func (p *engineDetailPage) listRows() int {
	rows := p.h - 2
	if p.filt.view() != "" {
		rows--
	}
	return max(rows, 1)
}

func (p *engineDetailPage) enter() (Page, tea.Cmd) {
	cl, m := p.cl, p.m
	if p.isKV() {
		if p.tab != 1 {
			return p, nil
		}
		vis := p.visibleEntries()
		if p.ecur >= len(vis) {
			return p, nil
		}
		e := vis[p.ecur]
		if e.IsDir {
			return p, Push(newKVPage(cl, m, e.Name))
		}
		return p, Push(newDetailPage(
			"secret: "+m.Path+e.Name,
			func() (string, []pageLink, error) {
				data, meta, err := vault.ReadKV(cl, m, e.Name, 0)
				if err != nil {
					return "", nil, err
				}
				return secretDoc(data, meta), nil, nil
			},
		))
	}
	// Role group tab
	g := p.groupTab()
	if g == nil {
		return p, nil
	}
	vis := p.visibleGroupRoles(g)
	if g.cur >= len(vis) {
		return p, nil
	}
	name := vis[g.cur]
	endpoint, label := g.endpoint, g.label
	dp := newDetailPage(
		fmt.Sprintf("%s: %s", label, name),
		func() (string, []pageLink, error) {
			data, err := vault.ReadRole(cl, m.Path, endpoint, name)
			if err != nil {
				return "", nil, err
			}
			return formatMap(data), policyLinks(cl, extractPolicies(data)), nil
		},
	)
	dp.linkTitle = "linked policies"
	return p, Push(dp)
}

// layout recomputes viewport geometry and re-wraps its content.
func (p *engineDetailPage) layout() {
	p.vp.Width = p.w
	p.vp.Height = p.h - 1 // below the tab bar
	p.vp.SetContent(wrapText(p.cfgText, p.w))
	p.evp.Width = p.w
	p.evp.Height = p.h - 1
	p.evp.SetContent(wrapText(p.ecfgText, p.w))
}

func (p *engineDetailPage) View() string {
	var b strings.Builder
	b.WriteString(p.tabBar())
	b.WriteString("\n")
	switch {
	case p.tab == 0:
		b.WriteString(p.viewConfig())
	case p.hasEngineConfig() && p.tab == 1:
		b.WriteString(p.viewEngineConfig())
	case p.isKV():
		b.WriteString(p.viewEntries())
	default:
		b.WriteString(p.viewGroup())
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
	s := strings.Join(parts, " ") + "   " + dimStyle.Render(info)
	if p.isKV() && p.tab == 1 {
		s += listInfo(p.ecur, len(p.entries), p.listRows(), "keys")
	} else if g := p.groupTab(); g != nil {
		s += listInfo(g.cur, len(g.roles), p.listRows(), strings.ToLower(g.label))
	}
	return s
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

func (p *engineDetailPage) viewEngineConfig() string {
	if p.ecfgLoading {
		return loadingStyle.Render("loading engine config ...")
	}
	if p.ecfgText == "" {
		return dimStyle.Render("no engine config data returned")
	}
	return p.evp.View()
}

// visibleGroupRoles returns the role names of one group tab matching
// the filter.
func (p *engineDetailPage) visibleGroupRoles(g *roleGroupState) []string {
	if p.filt.empty() {
		return g.roles
	}
	var out []string
	for _, r := range g.roles {
		if p.filt.match(r) {
			out = append(out, r)
		}
	}
	return out
}

// visibleEntries returns the Browse-tab entries matching the filter.
func (p *engineDetailPage) visibleEntries() []vault.KVEntry {
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

func (p *engineDetailPage) viewGroup() string {
	g := p.groupTab()
	if g == nil {
		return dimStyle.Render("(nothing here)")
	}
	what := strings.ToLower(g.label)
	if g.loading {
		return loadingStyle.Render("loading " + what + " ...")
	}
	if g.err != nil {
		return errStyle.Render("error listing " + what + ": " + g.err.Error())
	}
	if len(g.roles) == 0 {
		return dimStyle.Render("no " + what + " found")
	}
	vis := p.visibleGroupRoles(g)
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.listRows()
	g.cur, g.off = window(g.cur, g.off, len(vis), rows)
	padW := 0
	for _, r := range vis {
		if len(r) > padW {
			padW = len(r)
		}
	}
	padW += 2
	var b strings.Builder
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n\n")
	}
	for i := g.off; i < min(g.off+rows, len(vis)); i++ {
		name := vis[i]
		label := fmt.Sprintf("%-*s", padW, name)
		ann := ""
		if a := g.ann[name]; a != "" {
			ann = dimStyle.Render("-> " + a)
		}
		if i == g.cur {
			row := "  " + strings.TrimRight(label, " ")
			if a := g.ann[name]; a != "" {
				row += " -> " + a
			}
			b.WriteString(selBar(p.w, row) + "\n")
			continue
		}
		b.WriteString("  " + p.filt.highlight(label, itemStyle) + ann + "\n")
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
	vis := p.visibleEntries()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.listRows()
	p.ecur, p.eoff = window(p.ecur, p.eoff, len(vis), rows)
	var b strings.Builder
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n\n")
	}
	for i := p.eoff; i < min(p.eoff+rows, len(vis)); i++ {
		e := vis[i]
		if i == p.ecur {
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

func (p *engineDetailPage) Header() string {
	return fmt.Sprintf("engines / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *engineDetailPage) Typing() bool { return p.filt.typing() }

func (p *engineDetailPage) Loading() bool {
	switch {
	case p.tab == 0:
		return p.cfgLoading
	case p.hasEngineConfig() && p.tab == 1:
		return p.ecfgLoading
	case p.isKV():
		return p.entLoading
	default:
		return p.groupTab().loading
	}
}

func (p *engineDetailPage) Hints() string {
	return "left/right tabs  up/down select  enter open  v versions  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

// engineConfigDoc reads each engine config endpoint and renders them as
// labeled sections of one document.
func engineConfigDoc(cl *vaultapi.Client, mountPath string, endpoints []string) string {
	var b strings.Builder
	for i, ep := range endpoints {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(dimStyle.Render("-- "+mountPath+ep+" --") + "\n")
		data, err := vault.ReadEngineConfig(cl, mountPath+ep)
		switch {
		case err != nil:
			b.WriteString(errStyle.Render("error: " + err.Error()))
		case len(data) == 0:
			b.WriteString(dimStyle.Render("(no data returned)"))
		default:
			b.WriteString(formatMap(data))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
