package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// roleCfg is one role-like item's configuration (an auth role, a
// transit key, an AWS role, ...), fetched lazily for the expansion
// pane of the auth and engine pages.
type roleCfg struct {
	loading bool
	err     error
	data    map[string]interface{}
}

// roleCfgMsg delivers one role-like item's configuration. key is the
// group endpoint plus the item name: the expansion cache's key.
type roleCfgMsg struct {
	path string // full mount path prefix; guards stale messages
	key  string // endpoint + "/" + name
	data map[string]interface{}
	err  error
}

// roleCfgFetch reads one role-like item's configuration. mountPath is
// the full API prefix of the mount ("transit/", "auth/jwt/").
func roleCfgFetch(cl *vaultapi.Client, mountPath, endpoint, name string) tea.Cmd {
	return func() tea.Msg {
		data, err := vault.ReadRole(cl, mountPath, endpoint, name)
		return roleCfgMsg{path: mountPath, key: endpoint + "/" + name, data: data, err: err}
	}
}

// truncateLines caps the expansion pane at area lines, replacing the
// overflow with a dim indicator: rich documents elide rather than
// push the layout around.
func truncateLines(lines []string, area int) []string {
	if area < 1 {
		area = 1
	}
	if len(lines) > area {
		return append(lines[:area-1:area-1], dimStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-area+1)))
	}
	return lines
}

// expansionArea is the number of lines available to an expansion pane:
// the page height less the title, blank line, separator and one spare
// line, less the list rows actually shown. Short lists leave the pane
// the space their window would have taken; a small floor keeps a
// summary visible on tiny terminals.
func expansionArea(h, rowsShown int, filterLine bool) int {
	used := 4 + rowsShown
	if filterLine {
		used++
	}
	return max(h-used, 4)
}

// cfgExpansionLines renders a role-like item's summary for the
// expansion pane: the policies it grants, then its scalar fields.
// The full document stays one enter away.
func cfgExpansionLines(name string, cfg *roleCfg) []string {
	lines := []string{titleStyle.Render(name)}
	if cfg == nil {
		return lines
	}
	if cfg.loading {
		return append(lines, dimStyle.Render("loading ..."))
	}
	if cfg.err != nil {
		return append(lines, dimStyle.Render("error: "+cfg.err.Error()))
	}
	if pols := extractPolicies(cfg.data); len(pols) > 0 {
		lines = append(lines, keyStyle.Render("policies: ")+valStyle.Render(strings.Join(pols, ", ")))
	}
	keys := make([]string, 0, len(cfg.data))
	for k := range cfg.data {
		if k == "policies" || k == "token_policies" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, keyStyle.Render(k+": ")+cfgValue(cfg.data[k]))
	}
	return lines
}

// newRoleDetailPage builds the full-document page for one role-like
// item, with its linked policies as cross-reference links.
func newRoleDetailPage(cl *vaultapi.Client, mountPath, endpoint, label, name string) *detailPage {
	dp := newDetailPage(
		fmt.Sprintf("%s: %s", label, name),
		func() (string, []pageLink, error) {
			data, err := vault.ReadRole(cl, mountPath, endpoint, name)
			if err != nil {
				return "", nil, err
			}
			return formatMap(data), policyLinks(cl, extractPolicies(data)), nil
		},
	)
	dp.linkTitle = "linked policies"
	return dp
}

// roleListPage shows one role group's items (transit keys, PKI roles,
// AWS static roles, ...) as a single list; the cursor's item expands
// at the bottom with its config, fetched lazily. The engine page
// pushes one per group.
type roleListPage struct {
	cl        *vaultapi.Client
	mountPath string // full API prefix, e.g. "transit/"
	endpoint  string // list endpoint, e.g. "keys"
	label     string // group label, e.g. "Keys"
	crumb     string // Header() breadcrumb

	w, h int
	filt filter

	loading  bool
	err      error
	roles    []string
	cfgs     map[string]*roleCfg
	cur, off int
}

func newRoleListPage(cl *vaultapi.Client, mountPath, endpoint, label, crumb string) *roleListPage {
	return &roleListPage{
		cl:        cl,
		mountPath: mountPath,
		endpoint:  endpoint,
		label:     label,
		crumb:     crumb,
		loading:   true,
		cfgs:      map[string]*roleCfg{},
	}
}

func (p *roleListPage) Init() tea.Cmd {
	cl, mp, ep := p.cl, p.mountPath, p.endpoint
	return func() tea.Msg {
		roles, err := vault.ListRoles(cl, mp, ep)
		return rolesLoadedMsg{path: mp, endpoint: ep, roles: roles, err: err}
	}
}

func (p *roleListPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case rolesLoadedMsg:
		if msg.path != p.mountPath || msg.endpoint != p.endpoint {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.roles = msg.roles
		p.cur, p.off = 0, 0
		// The cursor lands on the first item with no keystroke to
		// trigger the prefetch; fetch it so the expansion pane fills.
		return p, p.ensureCfg()
	case roleCfgMsg:
		if msg.path != p.mountPath {
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

func (p *roleListPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
		np := newRoleListPage(p.cl, p.mountPath, p.endpoint, p.label, p.crumb)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			cl, mp, ep, label := p.cl, p.mountPath, p.endpoint, p.label
			return p, Push(newRoleDetailPage(cl, mp, ep, label, vis[p.cur]))
		}
	}
	return p, p.ensureCfg()
}

// visible returns the item names matching the filter.
func (p *roleListPage) visible() []string {
	if p.filt.empty() {
		return p.roles
	}
	var out []string
	for _, r := range p.roles {
		if p.filt.match(r) {
			out = append(out, r)
		}
	}
	return out
}

func (p *roleListPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

// ensureCfg prefetches the selected item's configuration for the
// expansion pane; already-cached items are not re-fetched.
func (p *roleListPage) ensureCfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) {
		return nil
	}
	name := vis[p.cur]
	key := p.endpoint + "/" + name
	if _, ok := p.cfgs[key]; ok {
		return nil
	}
	p.cfgs[key] = &roleCfg{loading: true}
	return roleCfgFetch(p.cl, p.mountPath, p.endpoint, name)
}

func (p *roleListPage) View() string {
	what := strings.ToLower(p.label)
	if p.loading {
		return loadingStyle.Render("loading " + what + " ...")
	}
	if p.err != nil {
		return errStyle.Render("error listing " + what + ": " + p.err.Error())
	}
	if len(p.roles) == 0 {
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
	b.WriteString("  " + itemStyle.Render(p.mountPath) + "  " + dimStyle.Render(p.label) + listInfo(p.cur, len(vis), rows, what) + "\n")
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
	for _, l := range truncateLines(cfgExpansionLines(vis[p.cur], p.cfgs[p.endpoint+"/"+vis[p.cur]]), expansionArea(p.h, shown, fl != "")) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *roleListPage) Header() string { return p.crumb }

func (p *roleListPage) Typing() bool { return p.filt.typing() }

func (p *roleListPage) Loading() bool { return p.loading }

func (p *roleListPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

// cfgValue renders one config value for the expansion pane: scalars
// and plain lists inline; nested structures collapse to a placeholder
// (the full document shows them properly).
func cfgValue(v interface{}) string {
	switch tv := v.(type) {
	case map[string]interface{}:
		return dimStyle.Render("(object)")
	case []interface{}:
		if hasMapElem(tv) {
			return dimStyle.Render("(list)")
		}
		return styledValue(tv)
	default:
		return styledValue(v)
	}
}
