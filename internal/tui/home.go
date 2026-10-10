package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"
)

type homePage struct {
	cl    *vaultapi.Client
	items []string
	cur   int
	w, h  int
}

func newHomePage(cl *vaultapi.Client) *homePage {
	return &homePage{
		cl:    cl,
		items: []string{"Secret Engines", "ACL Policies", "Auth Methods"},
	}
}

func (p *homePage) Init() tea.Cmd { return nil }

func (p *homePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cur > 0 {
				p.cur--
			}
		case "down", "j":
			if p.cur < len(p.items)-1 {
				p.cur++
			}
		case "enter":
			switch p.cur {
			case 0:
				return p, Push(newEnginesPage(p.cl))
			case 1:
				return p, Push(newPoliciesPage(p.cl))
			case 2:
				return p, Push(newAuthPage(p.cl))
			}
		}
	}
	return p, nil
}

func (p *homePage) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("HashiCorp Vault"))
	b.WriteString("\n\n")
	for i, it := range p.items {
		marker := "  "
		style := itemStyle
		if i == p.cur {
			marker = "> "
			style = selStyle
		}
		b.WriteString(style.Render(marker+it) + "\n")
	}
	return b.String()
}

func (p *homePage) Header() string { return "home" }

func (p *homePage) Hints() string {
	return "up/down select  enter open  q quit"
}
