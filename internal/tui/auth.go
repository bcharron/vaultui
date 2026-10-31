package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// authPage lists the enabled auth methods.
type authPage struct {
	cl     *vaultapi.Client
	mounts []vault.Mount
	listPage[vault.Mount]
}

type authLoadedMsg struct {
	mounts []vault.Mount
	err    error
}

func newAuthPage(cl *vaultapi.Client) *authPage {
	p := &authPage{cl: cl}
	p.loading = true
	p.items = func() []vault.Mount { return p.mounts }
	p.visible = func() []vault.Mount {
		if p.filt.empty() {
			return p.mounts
		}
		var out []vault.Mount
		for _, m := range p.mounts {
			if p.filt.match(m.Path) || p.filt.match(m.Type) {
				out = append(out, m)
			}
		}
		return out
	}
	p.row = func(m vault.Mount, sel bool) string {
		path := truncate(m.Path, 20)
		typ := truncate(m.Type, 16)
		desc := truncate(m.Description, max(p.w-40, 0))
		if sel {
			return selBar(p.w, fmt.Sprintf("  %-20s %-16s %s", path, typ, desc))
		}
		return fmt.Sprintf("  %s %s %s",
			p.filt.highlight(fmt.Sprintf("%-20s", path), itemStyle),
			p.filt.highlight(fmt.Sprintf("%-16s", typ), typeStyle),
			dimStyle.Render(desc))
	}
	p.onEnter = func(m vault.Mount) tea.Cmd {
		return Push(newAuthDetailPage(p.cl, m))
	}
	p.reload = p.Init
	p.title = dimStyle.Render(fmt.Sprintf("  %-20s %-16s %s", "PATH", "TYPE", "DESCRIPTION"))
	p.noun = "auth methods"
	p.headLines = 2
	p.loadingText = "loading auth methods ..."
	p.emptyText = "no auth methods enabled"
	p.errPrefix = "error: "
	return p
}

func (p *authPage) Init() tea.Cmd {
	cl := p.cl
	return func() tea.Msg {
		m, err := vault.ListAuthMounts(cl)
		return authLoadedMsg{mounts: m, err: err}
	}
}

func (p *authPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case authLoadedMsg:
		p.mounts = msg.mounts
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *authPage) Header() string { return "auth methods" }

func (p *authPage) Hints() string {
	return "up/down select  enter open auth method  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}

// authRoleTabLabel names the auth method's role-like items per auth
// type.
func authRoleTabLabel(authType string) string {
	switch authType {
	case "userpass", "okta":
		return "Users"
	case "github":
		return "Teams"
	case "ldap":
		return "Groups"
	default:
		return "Roles"
	}
}

// authDetailPage is the auth method's own screen, mirroring the engine
// page: a row opening the mount's tune config, and one row for the
// method's role-like items, which opens their own list screen. The
// selected row's summary expands at the bottom.
type authDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	filt filter

	cfgLoading bool
	cfgText    string

	// acfgEndpoints are the auth method's own config endpoints (jwt
	// config, cert config+crl/config, aws config/root, ...); non-empty
	// only for auth types that have them, and shown as a second
	// synthetic row whose expansion shows the rendered document,
	// fetched lazily when the row is first selected.
	acfgEndpoints []string
	acfgFetched   bool
	acfgLoading   bool
	acfgText      string

	rolesLoading bool
	rolesErr     error
	roles        []string
	roleEndpoint string

	cur, off int
}

type authTuneLoadedMsg struct {
	path string
	kv   []vault.KV
	err  error
}

func newAuthDetailPage(cl *vaultapi.Client, m vault.Mount) *authDetailPage {
	return &authDetailPage{
		cl:            cl,
		m:             m,
		cfgLoading:    true,
		acfgEndpoints: vault.AuthConfigEndpoints(m.Type),
		rolesLoading:  true,
		roleEndpoint:  vault.AuthRoleListEndpoint(m.Type),
	}
}

func (p *authDetailPage) Init() tea.Cmd {
	cl, m := p.cl, p.m
	return tea.Batch(
		func() tea.Msg {
			kv, err := vault.AuthTune(cl, m.Path)
			return authTuneLoadedMsg{path: m.Path, kv: kv, err: err}
		},
		func() tea.Msg {
			roles, err := vault.ListRoles(cl, "auth/"+m.Path, p.roleEndpoint)
			return rolesLoadedMsg{path: m.Path, endpoint: p.roleEndpoint, roles: roles, err: err}
		},
	)
}

func (p *authDetailPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case authTuneLoadedMsg:
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
		p.acfgLoading = false
		p.acfgText = msg.text
	case rolesLoadedMsg:
		if msg.path != p.m.Path || msg.endpoint != p.roleEndpoint {
			return p, nil
		}
		p.rolesLoading = false
		p.rolesErr = msg.err
		p.roles = msg.roles
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *authDetailPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
		np := newAuthDetailPage(p.cl, p.m)
		np.w, np.h = p.w, p.h
		return np, np.Init()
	case "enter":
		if p.cur < len(vis) {
			if cmd := p.enter(vis[p.cur]); cmd != nil {
				return p, tea.Batch(cmd, p.ensureAcfg())
			}
		}
	}
	return p, p.ensureAcfg()
}

// ensureAcfg fetches the auth method's own config once, when its row is
// first selected: the expansion pane's preview costs one request per
// auth method, only if it is ever inspected.
func (p *authDetailPage) ensureAcfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) || vis[p.cur].kind != rowEngineCfg || p.acfgFetched {
		return nil
	}
	p.acfgFetched = true
	p.acfgLoading = true
	cl, m, eps := p.cl, p.m, p.acfgEndpoints
	return func() tea.Msg {
		return engineCfgLoadedMsg{path: m.Path, text: engineConfigDoc(cl, "auth/"+m.Path, eps)}
	}
}

// enter opens the selected row: both config rows push their document;
// the roles row pushes the roles' own list screen.
func (p *authDetailPage) enter(r engineRow) tea.Cmd {
	cl, m := p.cl, p.m
	switch r.kind {
	case rowTune:
		return Push(newDetailPage(
			fmt.Sprintf("config: auth/%s", strings.TrimSuffix(m.Path, "/")),
			func() (string, []pageLink, error) {
				kv, err := vault.AuthTune(cl, m.Path)
				if err != nil {
					return "", nil, err
				}
				return formatPairs(kv), nil, nil
			},
		))
	case rowEngineCfg:
		eps := p.acfgEndpoints
		return Push(newDetailPage(
			fmt.Sprintf("auth config: auth/%s", strings.TrimSuffix(m.Path, "/")),
			func() (string, []pageLink, error) {
				return engineConfigDoc(cl, "auth/"+m.Path, eps), nil, nil
			},
		))
	}
	label := authRoleTabLabel(m.Type)
	crumb := fmt.Sprintf("auth / %s / %s", strings.TrimSuffix(m.Path, "/"), strings.ToLower(label))
	return Push(newRoleListPage(cl, "auth/"+m.Path, p.roleEndpoint, label, crumb))
}

// rows is the page's fixed row list: the tune config, the method's own
// config (when its type has one), and the roles.
func (p *authDetailPage) rows() []engineRow {
	rows := []engineRow{{kind: rowTune, label: engineTuneRowLabel}}
	if len(p.acfgEndpoints) > 0 {
		rows = append(rows, engineRow{kind: rowEngineCfg, label: authCfgRowLabel})
	}
	return append(rows, engineRow{kind: rowGroup, label: authRoleTabLabel(p.m.Type)})
}

// visible returns the rows matching the filter. The roles row matches
// on its label or on any role name, so filtering for a role keeps the
// row reachable.
func (p *authDetailPage) visible() []engineRow {
	if p.filt.empty() {
		return p.rows()
	}
	var out []engineRow
	for _, r := range p.rows() {
		if p.filt.match(r.label) {
			out = append(out, r)
			continue
		}
		if r.kind == rowGroup {
			for _, name := range p.roles {
				if p.filt.match(name) {
					out = append(out, r)
					break
				}
			}
		}
	}
	return out
}

func (p *authDetailPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

func (p *authDetailPage) View() string {
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
		if i == p.cur {
			// plain text so the highlight bar paints uniformly
			b.WriteString(selBar(p.w, "  "+p.rowPlain(vis[i])) + "\n")
			continue
		}
		b.WriteString("  " + p.rowText(vis[i]) + "\n")
	}

	b.WriteString(hRule(p.w) + "\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *authDetailPage) rowPlain(r engineRow) string {
	if r.kind == rowGroup {
		return r.label + p.rolesNote()
	}
	return r.label
}

func (p *authDetailPage) rowText(r engineRow) string {
	if r.kind == rowGroup {
		return itemStyle.Render(r.label) + dimStyle.Render(p.rolesNote())
	}
	return itemStyle.Render(r.label)
}

// rolesNote is the dim suffix of the roles row: its loading state or
// role count.
func (p *authDetailPage) rolesNote() string {
	switch {
	case p.rolesLoading:
		return "  ..."
	case p.rolesErr != nil:
		return "  (error)"
	case len(p.roles) == 0:
		return "  (none)"
	}
	return fmt.Sprintf("  %d", len(p.roles))
}

// expansion renders the selected row's summary below the rule: the
// tune pairs, the method's own config document, or the roles row's
// item names.
func (p *authDetailPage) expansion(vis []engineRow) []string {
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	area := expansionArea(p.h, min(p.rowsVisible(), max(len(vis)-p.off, 0)), p.filt.view() != "")
	switch r.kind {
	case rowTune:
		lines := []string{titleStyle.Render(engineTuneRowLabel), ""}
		if p.cfgLoading {
			lines = append(lines, dimStyle.Render("loading ..."))
		} else {
			lines = append(lines, strings.Split(p.cfgText, "\n")...)
		}
		return truncateLines(lines, area)
	case rowEngineCfg:
		lines := []string{titleStyle.Render(authCfgRowLabel), ""}
		if p.acfgLoading || !p.acfgFetched {
			lines = append(lines, dimStyle.Render("loading ..."))
		} else {
			lines = append(lines, strings.Split(p.acfgText, "\n")...)
		}
		return truncateLines(lines, area)
	}
	what := strings.ToLower(authRoleTabLabel(p.m.Type))
	lines := []string{titleStyle.Render(r.label), ""}
	switch {
	case p.rolesLoading:
		lines = append(lines, dimStyle.Render("loading "+what+" ..."))
	case p.rolesErr != nil:
		lines = append(lines, dimStyle.Render("error: "+p.rolesErr.Error()))
	default:
		for _, name := range p.roles {
			lines = append(lines, "  "+itemStyle.Render(name))
		}
		if len(p.roles) == 0 {
			lines = append(lines, dimStyle.Render("(none)"))
		}
	}
	return truncateLines(lines, area)
}

func (p *authDetailPage) Header() string {
	return fmt.Sprintf("auth / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *authDetailPage) Typing() bool { return p.filt.typing() }

func (p *authDetailPage) Loading() bool { return p.cfgLoading || p.acfgLoading || p.rolesLoading }

func (p *authDetailPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
