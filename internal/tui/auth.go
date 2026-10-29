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

// authDetailPage shows one auth method's role-like items in a single
// list: a synthetic row opening the mount's tune config on top, then
// the roles. The cursor's item expands at the bottom with its
// policies and config summary; enter opens the full document.
type authDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	filt filter

	cfgLoading bool
	cfgText    string // tune config, for the tune row's expansion

	rolesLoading bool
	rolesErr     error
	roles        []string

	cfgs         map[string]*roleCfg // role configs, fetched lazily
	roleEndpoint string              // list endpoint, to filter role messages
	cur, off     int
}

type authTuneLoadedMsg struct {
	path string
	kv   []vault.KV
	err  error
}

func newAuthDetailPage(cl *vaultapi.Client, m vault.Mount) *authDetailPage {
	return &authDetailPage{
		cl:           cl,
		m:            m,
		cfgLoading:   true,
		rolesLoading: true,
		cfgs:         map[string]*roleCfg{},
		roleEndpoint: vault.AuthRoleListEndpoint(m.Type),
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
	case rolesLoadedMsg:
		if msg.path != p.m.Path || msg.endpoint != p.roleEndpoint {
			return p, nil
		}
		p.rolesLoading = false
		p.rolesErr = msg.err
		p.roles = msg.roles
		p.cur, p.off = 0, 0
	case roleCfgMsg:
		if msg.path != "auth/"+p.m.Path {
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

func (p *authDetailPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
		np := newAuthDetailPage(p.cl, p.m)
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

// authRow is one row of the auth page's list: the synthetic tune row
// on top, then the roles.
type authRow struct {
	role string // empty for the tune row
}

// visible returns the rows matching the filter: the tune row when it
// matches, then the matching roles.
func (p *authDetailPage) visible() []authRow {
	var out []authRow
	if p.filt.match(engineTuneRowLabel) {
		out = append(out, authRow{})
	}
	for _, r := range p.roles {
		if p.filt.match(r) {
			out = append(out, authRow{role: r})
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

// ensureCfg prefetches the selected role's configuration for the
// expansion pane; already-cached roles are not re-fetched.
func (p *authDetailPage) ensureCfg() tea.Cmd {
	vis := p.visible()
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	if r.role == "" {
		return nil
	}
	if _, ok := p.cfgs[r.role]; ok {
		return nil
	}
	p.cfgs[r.role] = &roleCfg{loading: true}
	return roleCfgFetch(p.cl, "auth/"+p.m.Path, p.roleEndpoint, r.role)
}

// enter opens the selected row: the tune row and each role push
// ordinary stacked pages.
func (p *authDetailPage) enter(r authRow) tea.Cmd {
	cl, m := p.cl, p.m
	if r.role == "" {
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
	}
	name := r.role
	dp := newDetailPage(
		fmt.Sprintf("%s: %s", authRoleTabLabel(m.Type), name),
		func() (string, []pageLink, error) {
			data, err := vault.ReadRole(cl, "auth/"+m.Path, vault.AuthRoleListEndpoint(m.Type), name)
			if err != nil {
				return "", nil, err
			}
			return formatMap(data), policyLinks(cl, extractPolicies(data)), nil
		},
	)
	dp.linkTitle = "linked policies"
	return Push(dp)
}

func (p *authDetailPage) View() string {
	if p.rolesLoading {
		return loadingStyle.Render("loading " + strings.ToLower(authRoleTabLabel(p.m.Type)) + " ...")
	}
	if p.rolesErr != nil {
		return errStyle.Render("error listing " + strings.ToLower(authRoleTabLabel(p.m.Type)) + ": " + p.rolesErr.Error())
	}
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.rowsVisible()
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	var b strings.Builder
	b.WriteString("  " + itemStyle.Render(p.m.Path) + "  " + dimStyle.Render(p.m.Type+" "+p.m.Accessor) + listInfo(max(p.cur-1, 0), len(p.roles), rows, strings.ToLower(authRoleTabLabel(p.m.Type))) + "\n")
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

func (p *authDetailPage) row(r authRow, sel bool) string {
	if r.role == "" {
		if sel {
			return selBar(p.w, "  "+typeStyle.Render(engineTuneRowLabel))
		}
		return "  " + typeStyle.Render(engineTuneRowLabel)
	}
	if sel {
		return selBar(p.w, "  "+r.role)
	}
	return "  " + p.filt.highlight(r.role, itemStyle)
}

// expansion renders the selected row's summary below the rule: the
// tune pairs for the config row, the role's config otherwise.
func (p *authDetailPage) expansion(vis []authRow) []string {
	if p.cur >= len(vis) {
		return nil
	}
	r := vis[p.cur]
	area := max(p.h-p.h/2-4, 4)
	if r.role == "" {
		lines := []string{titleStyle.Render(engineTuneRowLabel)}
		if p.cfgLoading {
			lines = append(lines, dimStyle.Render("loading ..."))
		} else {
			lines = append(lines, strings.Split(p.cfgText, "\n")...)
		}
		return truncateLines(lines, area)
	}
	return truncateLines(cfgExpansionLines(r.role, p.cfgs[r.role]), area)
}

func (p *authDetailPage) Header() string {
	return fmt.Sprintf("auth / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *authDetailPage) Typing() bool { return p.filt.typing() }

func (p *authDetailPage) Loading() bool { return p.rolesLoading || p.cfgLoading }

func (p *authDetailPage) Hints() string {
	return "up/down select  enter open  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
