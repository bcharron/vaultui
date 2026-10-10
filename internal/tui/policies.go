package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"vaultui/internal/vault"
)

type policiesPage struct {
	cl      *vaultapi.Client
	names   []string
	cur     int
	off     int
	loading bool
	err     error
	w, h    int
}

type policiesLoadedMsg struct {
	names []string
	err   error
}

func newPoliciesPage(cl *vaultapi.Client) *policiesPage {
	return &policiesPage{cl: cl, loading: true}
}

func (p *policiesPage) Init() tea.Cmd {
	cl := p.cl
	return func() tea.Msg {
		names, err := vault.ListPolicies(cl)
		return policiesLoadedMsg{names: names, err: err}
	}
}

// newPolicyDetailPage builds the viewer for one policy's rules.
func newPolicyDetailPage(cl *vaultapi.Client, name string) Page {
	return newDetailPage(
		"policy: "+name,
		func() (string, []pageLink, error) {
			rules, err := vault.ReadPolicy(cl, name)
			return rules, nil, err
		},
	)
}

func (p *policiesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case policiesLoadedMsg:
		p.loading = false
		p.err = msg.err
		p.names = msg.names
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(p.names)-1 {
				p.cur++
			}
		case "enter":
			if p.cur < len(p.names) {
				return p, Push(newPolicyDetailPage(p.cl, p.names[p.cur]))
			}
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

func (p *policiesPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading policies ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	if len(p.names) == 0 {
		return dimStyle.Render("no policies visible")
	}
	rows := p.h - 2
	if rows < 1 {
		rows = 1
	}
	p.cur, p.off = window(p.cur, p.off, len(p.names), rows)
	var b strings.Builder
	b.WriteString(dimStyle.Render("  ACL POLICIES") + "\n\n")
	for i := p.off; i < min(p.off+rows, len(p.names)); i++ {
		if i == p.cur {
			b.WriteString(selStyle.Render("> "+p.names[i]) + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+p.names[i]) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *policiesPage) Header() string { return "policies" }

func (p *policiesPage) Hints() string {
	return "up/down select  enter view policy  r refresh  esc back  q quit"
}
