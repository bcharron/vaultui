package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// homeStatusMsg and homeHealthMsg carry the home screen's async status
// fetches (sys/seal-status, sys/health).
type homeStatusMsg struct {
	seal map[string]interface{}
	err  error
}

type homeHealthMsg struct {
	health map[string]interface{}
	err    error
}

type homePage struct {
	cl    *vaultapi.Client
	items []string
	cur   int
	w, h  int

	// statusPending counts the in-flight status fetches; the maps are
	// filled in as they arrive.
	statusPending int
	seal          map[string]interface{}
	sealErr       error
	health        map[string]interface{}
}

func newHomePage(cl *vaultapi.Client) *homePage {
	return &homePage{
		cl: cl,
		items: []string{
			"ACL Policies",
			"Auth Methods",
			"Identity",
			"Leases",
			"Secret Engines",
			"Server Status",
			"Token Info",
		},
		statusPending: 2,
	}
}

// Init fetches the server status in the background; the page renders
// immediately and the status line fills in when the data arrives.
func (p *homePage) Init() tea.Cmd {
	cl := p.cl
	return tea.Batch(
		func() tea.Msg {
			seal, err := vault.SealStatus(cl)
			return homeStatusMsg{seal: seal, err: err}
		},
		func() tea.Msg {
			health, err := vault.Health(cl)
			return homeHealthMsg{health: health, err: err}
		},
	)
}

func (p *homePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case homeStatusMsg:
		p.statusPending--
		p.seal = msg.seal
		p.sealErr = msg.err
	case homeHealthMsg:
		// Vault answers non-200 (as an error) when sealed or a standby;
		// the summary renders fine without the health data.
		p.statusPending--
		if msg.err == nil {
			p.health = msg.health
		}
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
				return p, Push(newPoliciesPage(p.cl))
			case 1:
				return p, Push(newAuthPage(p.cl))
			case 2:
				return p, Push(newIdentityPage(p.cl))
			case 3:
				return p, Push(newLeasesPage(p.cl, ""))
			case 4:
				return p, Push(newEnginesPage(p.cl))
			case 5:
				return p, Push(newServerStatusPage(p.cl))
			case 6:
				return p, Push(newTokenPage(p.cl))
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
	b.WriteString(p.statusLine())
	b.WriteString("\n\n")
	for i, it := range p.items {
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+it) + "\n")
		} else {
			b.WriteString("  " + itemStyle.Render(it) + "\n")
		}
	}
	return b.String()
}

// statusLine renders the one-line server summary shown above the menu:
// the curated headline while data is arriving, a dim placeholder until
// then, nothing when the server would not answer.
func (p *homePage) statusLine() string {
	if p.statusPending > 0 {
		return dimStyle.Render("loading status ...")
	}
	if s := statusSummary(p.seal, p.health); s != "" {
		return s
	}
	return dimStyle.Render("status unavailable")
}

func (p *homePage) Header() string { return "home" }

func (p *homePage) Typing() bool { return false }

func (p *homePage) Loading() bool { return p.statusPending > 0 }

func (p *homePage) Hints() string {
	return "up/down select  enter open  q quit"
}
