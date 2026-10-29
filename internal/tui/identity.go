package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type entitiesLoadedMsg struct {
	names []string
	err   error
}

type groupsLoadedMsg struct {
	names []string
	err   error
}

// identityPage shows what identity has - one row per group of items
// (entities, groups), each opening its own list screen. The selected
// row's item names preview in the bottom pane.
type identityPage struct {
	cl   *vaultapi.Client
	w, h int
	filt filter

	entities        []string
	entitiesLoading bool
	entitiesErr     error

	groups        []string
	groupsLoading bool
	groupsErr     error

	cur, off int
}

func newIdentityPage(cl *vaultapi.Client) *identityPage {
	return &identityPage{cl: cl, entitiesLoading: true, groupsLoading: true}
}

func (p *identityPage) Init() tea.Cmd {
	cl := p.cl
	return tea.Batch(
		func() tea.Msg {
			names, err := vault.ListEntities(cl)
			return entitiesLoadedMsg{names: names, err: err}
		},
		func() tea.Msg {
			names, err := vault.ListGroups(cl)
			return groupsLoadedMsg{names: names, err: err}
		},
	)
}

func (p *identityPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case entitiesLoadedMsg:
		p.entitiesLoading = false
		p.entitiesErr = msg.err
		p.entities = msg.names
	case groupsLoadedMsg:
		p.groupsLoading = false
		p.groupsErr = msg.err
		p.groups = msg.names
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *identityPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
		np := newIdentityPage(p.cl)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			if cmd := p.enter(vis[p.cur]); cmd != nil {
				return p, cmd
			}
		}
	}
	return p, nil
}

// enter opens the selected group's own list screen.
func (p *identityPage) enter(r engineRow) tea.Cmd {
	return Push(newIdentityListPage(p.cl, r.ep))
}

// rows is the page's fixed row list.
func (p *identityPage) rows() []engineRow {
	return []engineRow{
		{kind: rowGroup, label: "Entities", ep: "entities"},
		{kind: rowGroup, label: "Groups", ep: "groups"},
	}
}

// visible returns the rows matching the filter. A row matches on its
// label or on any of its item names, so filtering for an entity
// keeps its row reachable.
func (p *identityPage) visible() []engineRow {
	if p.filt.empty() {
		return p.rows()
	}
	var out []engineRow
	for _, r := range p.rows() {
		if p.filt.match(r.label) {
			out = append(out, r)
			continue
		}
		names, _, _ := p.groupNames(r.ep)
		for _, n := range names {
			if p.filt.match(n) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

func (p *identityPage) groupNames(ep string) ([]string, bool, error) {
	switch ep {
	case "entities":
		return p.entities, p.entitiesLoading, p.entitiesErr
	default:
		return p.groups, p.groupsLoading, p.groupsErr
	}
}

func (p *identityPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

func (p *identityPage) View() string {
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.rowsVisible()
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	var b strings.Builder
	b.WriteString("  " + dimStyle.Render("IDENTITY") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		r := vis[i]
		if i == p.cur {
			// plain text so the highlight bar paints uniformly
			b.WriteString(selBar(p.w, "  "+r.label+p.groupNote(r.ep)) + "\n")
			continue
		}
		b.WriteString("  " + itemStyle.Render(r.label) + dimStyle.Render(p.groupNote(r.ep)) + "\n")
	}

	b.WriteString(hRule(p.w) + "\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// groupNote is the dim suffix of a row: its loading state or item
// count.
func (p *identityPage) groupNote(ep string) string {
	names, loading, err := p.groupNames(ep)
	switch {
	case loading:
		return "  ..."
	case err != nil:
		return "  (error)"
	case len(names) == 0:
		return "  (none)"
	}
	return fmt.Sprintf("  %d", len(names))
}

// expansion previews the selected row's item names below the rule.
func (p *identityPage) expansion(vis []engineRow) []string {
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	area := expansionArea(p.h, min(p.rowsVisible(), max(len(vis)-p.off, 0)), p.filt.view() != "")
	names, loading, err := p.groupNames(r.ep)
	lines := []string{titleStyle.Render(r.label), ""}
	switch {
	case loading:
		lines = append(lines, dimStyle.Render("loading "+r.ep+" ..."))
	case err != nil:
		lines = append(lines, dimStyle.Render("error: "+err.Error()))
	default:
		for _, n := range names {
			lines = append(lines, "  "+itemStyle.Render(n))
		}
		if len(names) == 0 {
			lines = append(lines, dimStyle.Render("(none)"))
		}
	}
	return truncateLines(lines, area)
}

func (p *identityPage) Header() string { return "identity" }

func (p *identityPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

func (p *identityPage) Typing() bool { return p.filt.typing() }

func (p *identityPage) Loading() bool { return p.entitiesLoading || p.groupsLoading }

// identityListPage shows identity's entities or groups as a single
// list; the cursor's item expands at the bottom with its config,
// fetched lazily. The identity page pushes one per group.
type identityListPage struct {
	cl   *vaultapi.Client
	kind string // "entities" or "groups"
	w, h int
	filt filter

	loading  bool
	err      error
	names    []string
	cfgs     map[string]*roleCfg
	cur, off int
}

func newIdentityListPage(cl *vaultapi.Client, kind string) *identityListPage {
	return &identityListPage{
		cl:      cl,
		kind:    kind,
		loading: true,
		cfgs:    map[string]*roleCfg{},
	}
}

func (p *identityListPage) Init() tea.Cmd {
	cl, kind := p.cl, p.kind
	if kind == "entities" {
		return func() tea.Msg {
			names, err := vault.ListEntities(cl)
			return entitiesLoadedMsg{names: names, err: err}
		}
	}
	return func() tea.Msg {
		names, err := vault.ListGroups(cl)
		return groupsLoadedMsg{names: names, err: err}
	}
}

func (p *identityListPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case entitiesLoadedMsg:
		if p.kind != "entities" {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.names = msg.names
		p.cur, p.off = 0, 0
		// The cursor lands on the first item with no keystroke to
		// trigger the prefetch; fetch it so the expansion pane fills.
		return p, p.ensureCfg()
	case groupsLoadedMsg:
		if p.kind != "groups" {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.names = msg.names
		p.cur, p.off = 0, 0
		return p, p.ensureCfg()
	case roleCfgMsg:
		if msg.path != "identity/"+p.kind {
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

func (p *identityListPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
		np := newIdentityListPage(p.cl, p.kind)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			return p, Push(p.open(vis[p.cur]))
		}
	}
	return p, p.ensureCfg()
}

// open builds the full-document page for one item, with its policies
// as cross-reference links.
func (p *identityListPage) open(name string) Page {
	cl, kind := p.cl, p.kind
	if kind == "entities" {
		return identityDetail(cl, "entity: "+name, func() (map[string]interface{}, error) {
			return vault.ReadEntity(cl, name)
		})
	}
	return identityDetail(cl, "group: "+name, func() (map[string]interface{}, error) {
		return vault.ReadGroup(cl, name)
	})
}

// visible returns the item names matching the filter.
func (p *identityListPage) visible() []string {
	if p.filt.empty() {
		return p.names
	}
	var out []string
	for _, n := range p.names {
		if p.filt.match(n) {
			out = append(out, n)
		}
	}
	return out
}

func (p *identityListPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

// ensureCfg prefetches the selected item's configuration for the
// expansion pane; already-cached items are not re-fetched.
func (p *identityListPage) ensureCfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) {
		return nil
	}
	name := vis[p.cur]
	key := p.kind + "/" + name
	if _, ok := p.cfgs[key]; ok {
		return nil
	}
	p.cfgs[key] = &roleCfg{loading: true}
	cl, kind := p.cl, p.kind
	return func() tea.Msg {
		var (
			data map[string]interface{}
			err  error
		)
		if kind == "entities" {
			data, err = vault.ReadEntity(cl, name)
		} else {
			data, err = vault.ReadGroup(cl, name)
		}
		return roleCfgMsg{path: "identity/" + kind, key: key, data: data, err: err}
	}
}

func (p *identityListPage) View() string {
	what := p.kind
	if p.loading {
		return loadingStyle.Render("loading " + what + " ...")
	}
	if p.err != nil {
		return errStyle.Render("error listing " + what + ": " + p.err.Error())
	}
	if len(p.names) == 0 {
		return dimStyle.Render("no " + what + " found")
	}
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.rowsVisible()
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)
	shown := min(rows, max(len(vis)-p.off, 0))
	fl := p.filt.view()

	var b strings.Builder
	b.WriteString("  " + itemStyle.Render("identity") + "  " + dimStyle.Render(what) + listInfo(p.cur, len(vis), rows, what) + "\n")
	if fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+vis[i]) + "\n")
			continue
		}
		b.WriteString("  " + p.filt.highlight(vis[i], itemStyle) + "\n")
	}

	b.WriteString(hRule(p.w) + "\n")
	for _, l := range truncateLines(cfgExpansionLines(vis[p.cur], p.cfgs[p.kind+"/"+vis[p.cur]]), expansionArea(p.h, shown, fl != "")) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *identityListPage) Header() string { return "identity / " + p.kind }

func (p *identityListPage) Typing() bool { return p.filt.typing() }

func (p *identityListPage) Loading() bool { return p.loading }

func (p *identityListPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

// identityDetail builds a detail page for an entity/group, wiring its
// policies as navigable links.
func identityDetail(cl *vaultapi.Client, title string, read func() (map[string]interface{}, error)) Page {
	dp := newDetailPage(
		title,
		func() (string, []pageLink, error) {
			data, err := read()
			if err != nil {
				return "", nil, err
			}
			return formatMap(data), policyLinks(cl, extractPolicies(data)), nil
		},
	)
	dp.linkTitle = "linked policies"
	return dp
}
