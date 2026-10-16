package tui

import (
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

// identityPage shows identity entities and groups in two tabs, with
// drill-in detail and policy cross-links.
type identityPage struct {
	cl   *vaultapi.Client
	tab  int // 0 entities, 1 groups
	w, h int

	entities        []string
	ecur, eoff      int
	entitiesLoading bool
	entitiesErr     error

	groups        []string
	gcur, goff    int
	groupsLoading bool
	groupsErr     error
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
		p.ecur, p.eoff = 0, 0
	case groupsLoadedMsg:
		p.groupsLoading = false
		p.groupsErr = msg.err
		p.groups = msg.names
		p.gcur, p.goff = 0, 0
	case tea.KeyMsg:
		return p.updateKeys(msg)
	}
	return p, nil
}

func (p *identityPage) updateKeys(msg tea.KeyMsg) (Page, tea.Cmd) {
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
	case "up", "k":
		if p.tab == 0 && p.ecur > 0 {
			p.ecur--
		}
		if p.tab == 1 && p.gcur > 0 {
			p.gcur--
		}
		return p, nil
	case "down", "j":
		if p.tab == 0 && p.ecur < len(p.entities)-1 {
			p.ecur++
		}
		if p.tab == 1 && p.gcur < len(p.groups)-1 {
			p.gcur++
		}
		return p, nil
	case "enter":
		return p.enter()
	case "r":
		p.entitiesLoading = true
		p.groupsLoading = true
		p.entitiesErr = nil
		p.groupsErr = nil
		return p, p.Init()
	}
	return p, nil
}

func (p *identityPage) enter() (Page, tea.Cmd) {
	cl := p.cl
	if p.tab == 0 {
		if p.ecur >= len(p.entities) {
			return p, nil
		}
		name := p.entities[p.ecur]
		return p, Push(identityDetail(cl, "entity: "+name, func() (map[string]interface{}, error) {
			return vault.ReadEntity(cl, name)
		}))
	}
	if p.gcur >= len(p.groups) {
		return p, nil
	}
	name := p.groups[p.gcur]
	return p, Push(identityDetail(cl, "group: "+name, func() (map[string]interface{}, error) {
		return vault.ReadGroup(cl, name)
	}))
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

func (p *identityPage) View() string {
	var b strings.Builder
	for i, t := range []string{"Entities", "Groups"} {
		if i == p.tab {
			b.WriteString(activeTabStyle.Render("[" + t + "]"))
		} else {
			b.WriteString(dimStyle.Render(" " + t + " "))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n")
	if p.tab == 0 {
		b.WriteString(p.viewList(p.entities, p.entitiesLoading, p.entitiesErr, p.ecur, p.eoff, "entities"))
	} else {
		b.WriteString(p.viewList(p.groups, p.groupsLoading, p.groupsErr, p.gcur, p.goff, "groups"))
	}
	return b.String()
}

func (p *identityPage) viewList(names []string, loading bool, listErr error, cur, off int, what string) string {
	if loading {
		return loadingStyle.Render("loading " + what + " ...")
	}
	if listErr != nil {
		return errStyle.Render("error: " + listErr.Error())
	}
	if len(names) == 0 {
		return dimStyle.Render("no " + what + " found")
	}
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	cur, off = window(cur, off, len(names), rows)
	var b strings.Builder
	for i := off; i < min(off+rows, len(names)); i++ {
		if i == cur {
			b.WriteString(selStyle.Render("> "+names[i]) + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+names[i]) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *identityPage) Header() string { return "identity" }

func (p *identityPage) Hints() string {
	return "left/right tabs  up/down select  enter open  r refresh  esc back  q quit"
}

func (p *identityPage) Typing() bool { return false }
