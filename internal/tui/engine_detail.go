package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// engineDetailPage is the engine's own screen: a short list of what
// the engine has — the tune config, the engine's own config (when the
// type has one), and one row per role group, each opening the group's
// own list screen. The selected row's summary expands at the bottom.
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
	// that have them, and shown as a second synthetic row whose
	// expansion shows the rendered document, fetched lazily when the
	// row is first selected.
	ecfgEndpoints []string
	ecfgFetched   bool
	ecfgLoading   bool
	ecfgText      string

	// groups are the engine's role groups (Roles, Keys, Rolesets,
	// ...); each row opens the group's own screen.
	groups []groupState

	cur, off int
}

// groupState is one role group of the engine page's list.
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

type engineCfgLoadedMsg struct {
	path string
	text string
}

// engineRowKind distinguishes the rows of the engine page's list.
type engineRowKind int

const (
	rowTune engineRowKind = iota
	rowEngineCfg
	rowGroup
)

// engineRow is one row of the engine page's list.
type engineRow struct {
	kind  engineRowKind
	label string
	ep    string // role group endpoint
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
	}
	if m.Type == "kv" {
		// KV engines have no role endpoints; their single group row
		// opens the KV browser, and the count comes from the mount's
		// top-level entries.
		p.groups = append(p.groups, groupState{label: "Keys", loading: true})
	} else {
		for _, g := range vault.RoleGroups(m.Type) {
			p.groups = append(p.groups, groupState{
				label:    g.Label,
				endpoint: g.Endpoint,
				loading:  true,
			})
		}
	}
	return p
}

func (p *engineDetailPage) isKV() bool { return p.m.Type == "kv" }

func (p *engineDetailPage) Init() tea.Cmd {
	cl, m := p.cl, p.m
	cmds := []tea.Cmd{
		func() tea.Msg {
			kv, err := vault.Tune(cl, m.Path)
			return tuneLoadedMsg{path: m.Path, kv: kv, err: err}
		},
	}
	if p.isKV() {
		cmds = append(cmds, func() tea.Msg {
			entries, err := vault.ListKV(cl, m, "")
			return kvEntriesLoadedMsg{mount: m.Path, sub: "", entries: entries, err: err}
		})
	} else {
		for _, g := range p.groups {
			g := g
			cmds = append(cmds, func() tea.Msg {
				roles, err := vault.ListRoles(cl, m.Path, g.endpoint)
				return rolesLoadedMsg{path: m.Path, endpoint: g.endpoint, roles: roles, err: err}
			})
		}
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
	case engineCfgLoadedMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		p.ecfgLoading = false
		p.ecfgText = msg.text
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
	case kvEntriesLoadedMsg:
		if !p.isKV() || msg.mount != p.m.Path || msg.sub != "" || len(p.groups) == 0 {
			return p, nil
		}
		g := &p.groups[0]
		g.loading = false
		g.err = msg.err
		g.roles = make([]string, 0, len(msg.entries))
		for _, e := range msg.entries {
			g.roles = append(g.roles, e.Name)
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
		p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), false)
	case "pgdown":
		p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), true)
	case "r":
		np := newEngineDetailPage(p.cl, p.m)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			if cmd := p.enter(vis[p.cur]); cmd != nil {
				return p, tea.Batch(cmd, p.ensureEcfg())
			}
		}
	}
	return p, p.ensureEcfg()
}

// ensureEcfg fetches the engine config once, when its row is first
// selected: the expansion pane's preview costs one request per
// engine, only if it is ever inspected.
func (p *engineDetailPage) ensureEcfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) || vis[p.cur].kind != rowEngineCfg || p.ecfgFetched {
		return nil
	}
	p.ecfgFetched = true
	p.ecfgLoading = true
	cl, m, eps := p.cl, p.m, p.ecfgEndpoints
	return func() tea.Msg {
		return engineCfgLoadedMsg{path: m.Path, text: engineConfigDoc(cl, m.Path, eps)}
	}
}

// enter opens the selected row: both config rows push their document;
// a group row pushes the group's own list screen.
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
	case rowGroup:
		if p.isKV() {
			// the Keys row opens the KV browser
			return Push(newKVPage(cl, m, ""))
		}
		g := p.groupByEndpoint(r.ep)
		crumb := fmt.Sprintf("engines / %s / %s", strings.TrimSuffix(m.Path, "/"), strings.ToLower(g.label))
		return Push(newRoleListPage(cl, m.Path, g.endpoint, g.label, crumb))
	}
	return nil
}

// allRows is the unfiltered row list: the config rows, then one row
// per role group.
func (p *engineDetailPage) allRows() []engineRow {
	var rows []engineRow
	rows = append(rows, engineRow{kind: rowTune, label: engineTuneRowLabel})
	if p.hasEngineConfig() {
		rows = append(rows, engineRow{kind: rowEngineCfg, label: engineCfgRowLabel})
	}
	for _, g := range p.groups {
		rows = append(rows, engineRow{kind: rowGroup, label: g.label, ep: g.endpoint})
	}
	return rows
}

// visible returns the rows matching the filter. A group row matches on
// its label or on any of its items, so filtering for a key name keeps
// the group reachable.
func (p *engineDetailPage) visible() []engineRow {
	if p.filt.empty() {
		return p.allRows()
	}
	var out []engineRow
	for _, r := range p.allRows() {
		if p.filt.match(r.label) {
			out = append(out, r)
			continue
		}
		if r.kind == rowGroup {
			g := p.groupByEndpoint(r.ep)
			for _, name := range g.roles {
				if p.filt.match(name) {
					out = append(out, r)
					break
				}
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

	var b strings.Builder
	b.WriteString("  " + itemStyle.Render(p.m.Path) + "  " + dimStyle.Render(p.m.Type+" "+p.m.Accessor) + "\n")
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

func (p *engineDetailPage) row(r engineRow, sel bool) string {
	// The selected row renders plain so the highlight bar paints the
	// whole line: embedded per-segment colors would break the bar's
	// background at their ANSI resets.
	if sel {
		return selBar(p.w, "  "+p.rowPlain(r))
	}
	return "  " + p.rowText(r)
}

// rowPlain is the row's text without styling, for the selection bar.
func (p *engineDetailPage) rowPlain(r engineRow) string {
	if r.kind == rowGroup {
		return r.label + p.groupNote(r.ep)
	}
	return r.label
}

func (p *engineDetailPage) rowText(r engineRow) string {
	switch r.kind {
	case rowTune, rowEngineCfg:
		return itemStyle.Render(r.label)
	default:
		return itemStyle.Render(r.label) + dimStyle.Render(p.groupNote(r.ep))
	}
}

// groupNote is the dim suffix of a group row: its loading state or
// item count.
func (p *engineDetailPage) groupNote(ep string) string {
	g := p.groupByEndpoint(ep)
	switch {
	case g.loading:
		return "  ..."
	case g.err != nil:
		return "  (error)"
	case len(g.roles) == 0:
		return "  (none)"
	}
	return fmt.Sprintf("  %d", len(g.roles))
}

// expansion renders the selected row's summary below the rule: the
// tune pairs, the engine config document, or the group's item names.
func (p *engineDetailPage) expansion(vis []engineRow) []string {
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	area := expansionArea(p.h, min(p.rowsVisible(), max(len(vis)-p.off, 0)), p.filt.view() != "")
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
		lines := []string{titleStyle.Render(engineCfgRowLabel)}
		if p.ecfgLoading || !p.ecfgFetched {
			lines = append(lines, dimStyle.Render("loading ..."))
		} else {
			lines = append(lines, strings.Split(p.ecfgText, "\n")...)
		}
		return truncateLines(lines, area)
	default:
		g := p.groupByEndpoint(r.ep)
		lines := []string{titleStyle.Render(g.label)}
		switch {
		case g.loading:
			lines = append(lines, dimStyle.Render("loading ..."))
		case g.err != nil:
			lines = append(lines, dimStyle.Render("error: "+g.err.Error()))
		default:
			for _, name := range g.roles {
				lines = append(lines, "  "+itemStyle.Render(name))
			}
			if len(g.roles) == 0 {
				lines = append(lines, dimStyle.Render("(none)"))
			}
		}
		return truncateLines(lines, area)
	}
}

func (p *engineDetailPage) Header() string {
	return fmt.Sprintf("engines / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *engineDetailPage) Typing() bool { return p.filt.typing() }

func (p *engineDetailPage) Loading() bool {
	if p.cfgLoading || p.ecfgLoading {
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
