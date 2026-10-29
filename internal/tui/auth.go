package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// authPage lists the enabled auth methods.
type authPage struct {
	cl      *vaultapi.Client
	mounts  []vault.Mount
	filt    filter
	cur     int
	off     int
	loading bool
	err     error
	w, h    int
}

type authLoadedMsg struct {
	mounts []vault.Mount
	err    error
}

func newAuthPage(cl *vaultapi.Client) *authPage {
	return &authPage{cl: cl, loading: true}
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
		p.w, p.h = msg.w, msg.h
	case authLoadedMsg:
		p.loading = false
		p.err = msg.err
		p.mounts = msg.mounts
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
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
		case "enter":
			if p.cur < len(vis) {
				return p, Push(newAuthDetailPage(p.cl, vis[p.cur]))
			}
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

// visible returns the auth mounts matching the filter.
func (p *authPage) visible() []vault.Mount {
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

func (p *authPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading auth methods ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	if len(p.mounts) == 0 {
		return dimStyle.Render("no auth methods enabled")
	}
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.h - 2
	if fl := p.filt.view(); fl != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-20s %-16s %s", "PATH", "TYPE", "DESCRIPTION")) + listInfo(p.cur, len(vis), rows, "auth methods") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		m := vis[i]
		path := truncate(m.Path, 20)
		typ := truncate(m.Type, 16)
		desc := truncate(m.Description, max(p.w-40, 0))

		if i == p.cur {
			row := fmt.Sprintf("  %-20s %-16s %s", path, typ, desc)
			b.WriteString(selBar(p.w, row) + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %s %s\n",
			p.filt.highlight(fmt.Sprintf("%-20s", path), itemStyle),
			p.filt.highlight(fmt.Sprintf("%-16s", typ), typeStyle),
			dimStyle.Render(desc)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *authPage) Header() string { return "auth methods" }

func (p *authPage) Typing() bool { return p.filt.typing() }

func (p *authPage) Loading() bool { return p.loading }

func (p *authPage) Hints() string {
	return "up/down select  enter open auth method  / filter  r refresh  esc back  q quit"
}

// authRoleTabLabel names the second tab per auth type.
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

// authDetailPage shows an auth method's tune config and its
// roles/users/groups.
type authDetailPage struct {
	cl   *vaultapi.Client
	m    vault.Mount
	w, h int
	tab  int // 0 config, 1 roles
	filt filter

	cfgText    string
	cfgLoading bool
	cfgErr     error
	vp         viewport.Model

	rolesLoading bool
	rolesErr     error
	roles        []string
	rcur, roff   int
	rolePol      map[string][]string
	roleEndpoint string // full list endpoint, to filter role messages
}

type rolePoliciesMsg struct {
	path     string
	role     string
	policies []string
	err      error
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
		rolePol:      map[string][]string{},
		roleEndpoint: "auth/" + m.Path + vault.AuthRoleListEndpoint(m.Type),
		vp:           viewport.New(0, 0),
	}
}

func (p *authDetailPage) Init() tea.Cmd {
	cl, m := p.cl, p.m
	endpoint := p.roleEndpoint
	return tea.Batch(
		func() tea.Msg {
			kv, err := vault.AuthTune(cl, m.Path)
			return authTuneLoadedMsg{path: m.Path, kv: kv, err: err}
		},
		func() tea.Msg {
			roles, err := vault.ListRoles(cl, "auth/"+m.Path, vault.AuthRoleListEndpoint(m.Type))
			return rolesLoadedMsg{path: m.Path, endpoint: endpoint, roles: roles, err: err}
		},
	)
}

// loadRolePolicies reads each listed role's config (capped) so the
// Roles tab can show the policies each role links to.
func (p *authDetailPage) loadRolePolicies() tea.Cmd {
	cl, m := p.cl, p.m
	endpoint := vault.AuthRoleListEndpoint(m.Type)
	names := p.roles
	if len(names) > 50 {
		names = names[:50]
	}
	cmds := make([]tea.Cmd, 0, len(names))
	for _, name := range names {
		role := name
		cmds = append(cmds, func() tea.Msg {
			data, err := vault.ReadRole(cl, "auth/"+m.Path, endpoint, role)
			if err != nil {
				return rolePoliciesMsg{path: m.Path, role: role, err: err}
			}
			return rolePoliciesMsg{path: m.Path, role: role, policies: extractPolicies(data)}
		})
	}
	return tea.Batch(cmds...)
}

func (p *authDetailPage) tabs() []string {
	return []string{"Config", authRoleTabLabel(p.m.Type)}
}

func (p *authDetailPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
		p.layout()
	case authTuneLoadedMsg:
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
		if msg.path != p.m.Path || msg.endpoint != p.roleEndpoint {
			return p, nil
		}
		p.rolesLoading = false
		p.rolesErr = msg.err
		p.roles = msg.roles
		p.rcur, p.roff = 0, 0
		return p, p.loadRolePolicies()
	case rolePoliciesMsg:
		if msg.path != p.m.Path {
			return p, nil
		}
		if msg.err == nil && len(msg.policies) > 0 {
			p.rolePol[msg.role] = msg.policies
		}
		return p, nil
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *authDetailPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
	if p.filt.typing() {
		if p.filt.update(msg) {
			p.rcur, p.roff = 0, 0
			return p, nil
		}
	} else if msg.String() == "/" && p.tab == 1 {
		p.filt.active = true
		return p, nil
	}
	vis := p.visibleRoles()
	switch msg.String() {
	case "tab", "right", "l":
		if p.tab < 1 {
			p.tab++
		}
		return p, nil
	case "shift+tab", "left", "h":
		if p.tab > 0 {
			p.tab--
		}
		return p, nil
	case "r":
		np := newAuthDetailPage(p.cl, p.m)
		np.w, np.h = p.w, p.h
		np.tab = p.tab
		np.layout()
		return np, np.Init()
	case "up", "k":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
			return p, nil
		}
		if p.rcur > 0 {
			p.rcur--
		}
		return p, nil
	case "down", "j":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
			return p, nil
		}
		if p.rcur < len(vis)-1 {
			p.rcur++
		}
		return p, nil
	case "pgup", "pgdown", "home", "end", "g", "G":
		if p.tab == 0 {
			p.vp, _ = p.vp.Update(msg)
		}
		return p, nil
	case "enter":
		if p.tab == 1 && p.rcur < len(vis) {
			name := vis[p.rcur]
			cl, m := p.cl, p.m
			dp := newDetailPage(
				fmt.Sprintf("%s: %s", p.tabs()[1], name),
				func() (string, []pageLink, error) {
					data, err := vault.ReadRole(cl, "auth/"+m.Path, vault.AuthRoleListEndpoint(m.Type), name)
					if err != nil {
						return "", nil, err
					}
					return formatMap(data), policyLinks(cl, extractPolicies(data)), nil
				},
			)
			dp.linkTitle = "linked policies"
			return p, Push(dp)
		}
		return p, nil
	}
	return p, nil
}

func (p *authDetailPage) layout() {
	p.vp.Width = p.w
	p.vp.Height = p.h - 1
	p.vp.SetContent(wrapText(p.cfgText, p.w))
}

func (p *authDetailPage) View() string {
	var b strings.Builder
	b.WriteString(p.tabBar())
	b.WriteString("\n")
	if p.tab == 0 {
		b.WriteString(p.viewConfig())
	} else {
		b.WriteString(p.viewRoles())
	}
	return b.String()
}

func (p *authDetailPage) tabBar() string {
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
	if p.tab == 1 {
		s += listInfo(p.rcur, len(p.roles), max(p.h-3, 1), strings.ToLower(p.tabs()[1]))
	}
	return s
}

func (p *authDetailPage) viewConfig() string {
	if p.cfgLoading {
		return loadingStyle.Render("loading config ...")
	}
	if p.cfgErr != nil {
		return errStyle.Render("error reading sys/auth/" + p.m.Path + "tune: " + p.cfgErr.Error())
	}
	if p.cfgText == "" {
		return dimStyle.Render("no tune data returned")
	}
	return p.vp.View()
}

// visibleRoles returns the role names matching the filter.
func (p *authDetailPage) visibleRoles() []string {
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

func (p *authDetailPage) viewRoles() string {
	if p.rolesLoading {
		return loadingStyle.Render("loading roles ...")
	}
	if p.rolesErr != nil {
		return errStyle.Render("error listing roles: " + p.rolesErr.Error())
	}
	if len(p.roles) == 0 {
		return dimStyle.Render("no roles found")
	}
	vis := p.visibleRoles()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.h - 2
	if fl := p.filt.view(); fl != "" {
		rows--
	}
	if rows < 1 {
		rows = 1
	}
	p.rcur, p.roff = window(p.rcur, p.roff, len(vis), rows)
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
	for i := p.roff; i < min(p.roff+rows, len(vis)); i++ {
		name := vis[i]
		label := fmt.Sprintf("%-*s", padW, name)
		pol := ""
		if ps := p.rolePol[name]; len(ps) > 0 {
			pol = dimStyle.Render("-> " + strings.Join(ps, ", "))
		}
		if i == p.rcur {
			row := "  " + strings.TrimRight(label, " ")
			if ps := p.rolePol[name]; len(ps) > 0 {
				row += " -> " + strings.Join(ps, ", ")
			}
			b.WriteString(selBar(p.w, row) + "\n")
			continue
		}
		b.WriteString(itemStyle.Render("  "+label) + pol + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *authDetailPage) Header() string {
	return fmt.Sprintf("auth / %s", strings.TrimSuffix(p.m.Path, "/"))
}

func (p *authDetailPage) Typing() bool { return p.filt.typing() }

func (p *authDetailPage) Loading() bool {
	if p.tab == 0 {
		return p.cfgLoading
	}
	return p.rolesLoading
}

func (p *authDetailPage) Hints() string {
	return "left/right tabs  up/down select  enter open  / filter roles  r refresh  esc back  q quit"
}
