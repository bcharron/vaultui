package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type engineDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	filt filter

	// Tune config, fetched up front for the tune row's expansion.
	cfgLoading bool
	cfgText    string

	// ecfgEndpoints are the engine's own config endpoints (aws
	// config/root+lease, gcp config); non-empty only for engine types
	// that have them, and shown as a second synthetic row.
	ecfgEndpoints []string

	// groups are the role-like sections of the list (Roles, Keys,
	// Rolesets, ...), one per RoleGroup of the engine type.
	groups []groupState

	// cfgs caches role configs for the expansion pane, keyed by
	// endpoint + "/" + name and fetched lazily as the cursor moves.
	cfgs     map[string]*roleCfg
	cur, off int
}

// groupState is one role-like section of the engine page's list.
type groupState struct {
	label    string
	endpoint string
	loading  bool
	err      error
	roles    []string
}

type tuneLoadedMsg struct {
	path string
	kv   []vault.KV
	err  error
}

// engineRowKind distinguishes the rows of the engine page's single
// list: the synthetic config rows on top, the per-group section
// headers, and the role-like items.
type engineRowKind int

const (
	rowTune engineRowKind = iota
	rowEngineCfg
	rowHeader
	rowRole
)

// engineRow is one row of the engine page's list.
type engineRow struct {
	kind  engineRowKind
	label string // synthetic rows and section headers
	ep    string // role group endpoint
	name  string // role name
}

const (
	engineTuneRowLabel = "Config (tune)"
	engineCfgRowLabel  = "Engine Config"
)

func newEngineDetailPage(cl *vaultapi.Client, m vault.Mount) *engineDetailPage {
	p := &engineDetailPage{
		cl:            cl,
		m:             m,
		cfgLoading:    true,
		ecfgEndpoints: vault.EngineConfigEndpoints(m.Type),
		cfgs:          map[string]*roleCfg{},
	}
	for _, g := range vault.RoleGroups(m.Type) {
		p.groups = append(p.groups, groupState{
			label:    g.Label,
			endpoint: g.Endpoint,
			loading:  true,
		})
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
	for _, g := range p.groups {
		g := g
		cmds = append(cmds, func() tea.Msg {
			roles, err := vault.ListRoles(cl, m.Path, g.endpoint)
			return rolesLoadedMsg{path: m.Path, endpoint: g.endpoint, roles: roles, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (p *engineDetailPage) hasEngineConfig() bool { return len(p.ecfgEndpoints) > 0 }

func (p *engineDetailPage) groupByEndpoint(ep string) *groupState {
	for i := range p.groups {
		if p.groups[i].endpoint == ep {
			return &p.groups[i]
		}
	}
	return nil
}

func (p *engineDetailPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case tuneLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		p.cfgLoading = false
		if msg.err == nil {
			p.cfgText = formatPairs(msg.kv)
		}
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
	case roleCfgMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		if c := p.cfgs[msg.key]; c != nil {
			c.loading = false
			c.err = msg.err
			c.data = msg.data
		}
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *engineDetailPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
	if p.filt.typing() {
		if p.filt.update(msg) {
			p.cur, p.off = 0, 0
			return p, p.ensureCfg()
		}
	} else if msg.String() == "/" {
		p.filt.active = true
		return p, nil
	}
	vis := p.visible()
	switch msg.String() {
	case "up", "k":
		p.move(vis, -1)
	case "down", "j":
		p.move(vis, 1)
	case "pgup":
		p.seek(vis, pageJump(p.cur, p.rowsVisible(), len(vis), false), -1)
	case "pgdown":
		p.seek(vis, pageJump(p.cur, p.rowsVisible(), len(vis), true), 1)
	case "r":
		np := newEngineDetailPage(p.cl, p.m)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			if cmd := p.enter(vis[p.cur]); cmd != nil {
				return p, cmd
			}
		}
	}
	return p, p.ensureCfg()
}

// move steps the cursor by one, skipping group headers (they carry no
// item of their own).
func (p *engineDetailPage) move(vis []engineRow, step int) {
	for cur := p.cur; ; {
		cur += step
		if cur < 0 || cur >= len(vis) {
			return
		}
		if vis[cur].kind != rowHeader {
			p.cur = cur
			return
		}
	}
}

// seek lands the cursor on target, or the nearest item row in
// direction d when target is a group header.
func (p *engineDetailPage) seek(vis []engineRow, target, d int) {
	for cur := target; cur >= 0 && cur < len(vis); cur += d {
		if vis[cur].kind != rowHeader {
			p.cur = cur
			return
		}
	}
}

// ensureCfg prefetches the selected item's configuration for the
// expansion pane; already-cached items are not re-fetched.
func (p *engineDetailPage) ensureCfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	if r.kind != rowRole {
		return nil
	}
	key := r.ep + "/" + r.name
	if _, ok := p.cfgs[key]; ok {
		return nil
	}
	p.cfgs[key] = &roleCfg{loading: true}
	return roleCfgFetch(p.cl, p.m.Path, r.ep, r.name)
}

// enter opens the selected row: the synthetic rows and items push
// ordinary stacked pages; section headers do nothing.
func (p *engineDetailPage) enter(r engineRow) tea.Cmd {
	cl, m := p.cl, p.m
	switch r.kind {
	case rowTune:
		return Push(newDetailPage(
			fmt.Sprintf("config: %s", strings.TrimSuffix(m.Path, "/")),
			func() (string, []pageLink, error) {
				kv, err := vault.Tune(cl, m.Path)
				if err != nil {
					return "", nil, err
				}
				return formatPairs(kv), nil, nil
			},
		))
	case rowEngineCfg:
		eps := p.ecfgEndpoints
		return Push(newDetailPage(
			fmt.Sprintf("engine config: %s", strings.TrimSuffix(m.Path, "/")),
			func() (string, []pageLink, error) {
				return engineConfigDoc(cl, m.Path, eps), nil, nil
			},
		))
	case rowRole:
		endpoint, name := r.ep, r.name
		label := "role"
		if g := p.groupByEndpoint(r.ep); g != nil {
			label = g.label
		}
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
		return Push(dp)
	}
	return nil
}

// allRows is the unfiltered row list: the synthetic config rows, then
// each group's header and its items.
func (p *engineDetailPage) allRows() []engineRow {
	var rows []engineRow
	rows = append(rows, engineRow{kind: rowTune, label: engineTuneRowLabel})
	if p.hasEngineConfig() {
		rows = append(rows, engineRow{kind: rowEngineCfg, label: engineCfgRowLabel})
	}
	for _, g := range p.groups {
		rows = append(rows, engineRow{kind: rowHeader, label: g.label, ep: g.endpoint})
		for _, name := range g.roles {
			rows = append(rows, engineRow{kind: rowRole, ep: g.endpoint, name: name})
		}
	}
	return rows
}

// visible returns the rows matching the filter. A group header stays
// only while the group has a matching item.
func (p *engineDetailPage) visible() []engineRow {
	if p.filt.empty() {
		return p.allRows()
	}
	match := map[string]bool{}
	for _, g := range p.groups {
		for _, name := range g.roles {
			if p.filt.match(name) {
				match[g.endpoint] = true
			}
		}
	}
	var out []engineRow
	for _, r := range p.allRows() {
		switch r.kind {
		case rowHeader:
			if match[r.ep] {
				out = append(out, r)
			}
		case rowRole:
			if p.filt.match(r.name) {
				out = append(out, r)
			}
		default:
			if p.filt.match(r.label) {
				out = append(out, r)
			}
		}
	}
	return out
}

func (p *engineDetailPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

func (p *engineDetailPage) View() string {
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.rowsVisible()
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	total := 0
	for _, g := range p.groups {
		total += len(g.roles)
	}
	pos := 0
	for i := 0; i <= p.cur && i < len(vis); i++ {
		if vis[i].kind == rowRole {
			pos++
		}
	}

	var b strings.Builder
	b.WriteString("  " + itemStyle.Render(p.m.Path) + "  " + dimStyle.Render(p.m.Type+" "+p.m.Accessor) + listInfo(pos, total, rows, p.noun()) + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		b.WriteString(p.row(vis[i], i == p.cur) + "\n")
	}

	b.WriteString(hRule(p.w) + "\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// noun names the counted items: the group's own noun for single-group
// engines ("keys"), roles otherwise.
func (p *engineDetailPage) noun() string {
	if len(p.groups) == 1 {
		return strings.ToLower(p.groups[0].label)
	}
	return "roles"
}

func (p *engineDetailPage) row(r engineRow, sel bool) string {
	text := "  " + p.rowText(r)
	if sel {
		return selBar(p.w, text)
	}
	return text
}

func (p *engineDetailPage) rowText(r engineRow) string {
	switch r.kind {
	case rowTune, rowEngineCfg:
		return typeStyle.Render(r.label)
	case rowHeader:
		return dimStyle.Render(p.headerText(r.ep))
	default:
		return p.filt.highlight(r.name, itemStyle)
	}
}

// headerText renders a group header with its loading or empty state,
// so an empty group stays visible rather than silently disappearing.
func (p *engineDetailPage) headerText(ep string) string {
	g := p.groupByEndpoint(ep)
	switch {
	case g.loading:
		return g.label + " (loading ...)"
	case g.err != nil:
		return g.label + " (error)"
	case len(g.roles) == 0:
		return g.label + " (none)"
	}
	return g.label
}

// expansion renders the selected row's summary below the rule: the
// tune pairs for the config row, the item's config otherwise.
func (p *engineDetailPage) expansion(vis []engineRow) []string {
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	area := max(p.h-p.h/2-4, 4)
	switch r.kind {
	case rowTune:
		lines := []string{titleStyle.Render(engineTuneRowLabel)}
		if p.cfgLoading {
			lines = append(lines, dimStyle.Render("loading ..."))
		} else {
			lines = append(lines, strings.Split(p.cfgText, "\n")...)
		}
		return truncateLines(lines, area)
	case rowEngineCfg:
		return truncateLines([]string{
			titleStyle.Render(engineCfgRowLabel),
			dimStyle.Render("the engine's own configuration; enter to read"),
		}, area)
	case rowHeader:
		g := p.groupByEndpoint(r.ep)
		lines := []string{titleStyle.Render(g.label)}
		if len(g.roles) == 0 && !g.loading && g.err == nil {
			lines = append(lines, dimStyle.Render("(none)"))
		}
		return truncateLines(lines, area)
	default:
		return truncateLines(cfgExpansionLines(r.name, p.cfgs[r.ep+"/"+r.name]), area)
	}
}

func (p *engineDetailPage) Header() string {
	return fmt.Sprintf("engines / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *engineDetailPage) Typing() bool { return p.filt.typing() }

func (p *engineDetailPage) Loading() bool {
	if p.cfgLoading {
		return true
	}
	for _, g := range p.groups {
		if g.loading {
			return true
		}
	}
	return false
}

func (p *engineDetailPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
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
