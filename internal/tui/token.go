package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// tokenPage shows the authenticated token's own properties and lets the
// user check its capabilities on an arbitrary path.
type tokenPage struct {
	cl *vaultapi.Client

	ti textinput.Model

	lookupLoading bool
	lookupErr     error
	lookup        map[string]interface{}

	capsLoading bool
	capsPath    string
	caps        []string
	capsErr     error

	w, h int
}

type tokenLookupMsg struct {
	data map[string]interface{}
	err  error
}

type capsMsg struct {
	path string
	caps []string
	err  error
}

func newTokenPage(cl *vaultapi.Client) *tokenPage {
	ti := textinput.New()
	ti.Placeholder = "secret/data/apps/api"
	ti.CharLimit = 4096
	ti.Prompt = "path> "
	ti.Focus()
	return &tokenPage{cl: cl, ti: ti, lookupLoading: true}
}

func (p *tokenPage) Init() tea.Cmd {
	cl := p.cl
	return func() tea.Msg {
		data, err := vault.LookupSelf(cl)
		return tokenLookupMsg{data: data, err: err}
	}
}

func (p *tokenPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
		p.ti.Width = max(msg.w-8, 20)
	case tokenLookupMsg:
		p.lookupLoading = false
		p.lookupErr = msg.err
		p.lookup = msg.data
	case capsMsg:
		p.capsLoading = false
		p.capsPath = msg.path
		p.caps = msg.caps
		p.capsErr = msg.err
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if p.ti.Value() != "" {
				p.ti.SetValue("")
				return p, nil
			}
			return p, Pop()
		case "enter":
			path := strings.TrimSpace(p.ti.Value())
			if path == "" {
				return p, nil
			}
			p.capsLoading = true
			cl := p.cl
			return p, func() tea.Msg {
				caps, err := vault.CapabilitiesSelf(cl, path)
				return capsMsg{path: path, caps: caps, err: err}
			}
		}
		var cmd tea.Cmd
		p.ti, cmd = p.ti.Update(msg)
		return p, cmd
	}
	return p, nil
}

func (p *tokenPage) View() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("-- token (lookup-self) --"))
	b.WriteString("\n")
	if p.lookupLoading {
		b.WriteString(loadingStyle.Render("loading token info ..."))
	} else if p.lookupErr != nil {
		b.WriteString(errStyle.Render("error: " + p.lookupErr.Error()))
	} else {
		b.WriteString(formatMap(p.lookup))
	}
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("-- capabilities check --"))
	b.WriteString("\n")
	b.WriteString(p.ti.View())
	b.WriteString("\n")
	if p.capsLoading {
		b.WriteString(loadingStyle.Render("checking ..."))
	} else if p.capsErr != nil {
		b.WriteString(errStyle.Render("error: " + p.capsErr.Error()))
	} else if len(p.caps) > 0 {
		b.WriteString(dimStyle.Render("path: ") + valStyle.Render(p.capsPath))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("capabilities: ") + selStyle.Render(strings.Join(p.caps, ", ")))
	}
	return b.String()
}

func (p *tokenPage) Header() string { return "token info" }

func (p *tokenPage) Hints() string {
	return "enter check capabilities  esc clear input  ctrl+l redraw  q quit"
}

func (p *tokenPage) Typing() bool { return true }

func (p *tokenPage) Loading() bool { return p.lookupLoading || p.capsLoading }
