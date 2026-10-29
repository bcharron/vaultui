package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// policiesPage lists the ACL policies, with the synthetic union row on
// top.
type policiesPage struct {
	cl    *vaultapi.Client
	names []string
	listPage[string]
}

type policiesLoadedMsg struct {
	names []string
	err   error
}

// allPoliciesEntry is the synthetic first row of the policies list:
// it opens the union of every policy's paths.
const allPoliciesEntry = "All policies (union)"

func newPoliciesPage(cl *vaultapi.Client) *policiesPage {
	p := &policiesPage{cl: cl}
	p.loading = true
	p.items = func() []string { return p.names }
	p.visible = func() []string {
		var out []string
		if p.filt.match(allPoliciesEntry) {
			out = append(out, allPoliciesEntry)
		}
		if p.filt.empty() {
			return append(out, p.names...)
		}
		for _, n := range p.names {
			if p.filt.match(n) {
				out = append(out, n)
			}
		}
		return out
	}
	p.row = func(name string, sel bool) string {
		if sel {
			return selBar(p.w, "  "+name)
		}
		if name == allPoliciesEntry {
			return "  " + typeStyle.Render(name)
		}
		return "  " + p.filt.highlight(name, itemStyle)
	}
	p.onEnter = func(name string) tea.Cmd {
		if name == allPoliciesEntry {
			return Push(newAllPoliciesPage(p.cl, p.names))
		}
		return Push(newPolicyDetailPage(p.cl, name))
	}
	p.reload = p.Init
	p.title = "  " + dimStyle.Render("POLICIES")
	p.noun = "policies"
	p.headLines = 2
	p.loadingText = "loading policies ..."
	p.emptyText = "no policies visible"
	p.errPrefix = "error: "
	return p
}

func (p *policiesPage) Init() tea.Cmd {
	cl := p.cl
	return func() tea.Msg {
		names, err := vault.ListPolicies(cl)
		return policiesLoadedMsg{names: names, err: err}
	}
}

// newPolicyDetailPage builds the structured viewer for one policy's
// rules (path list with expansion; h shows the raw document).
func newPolicyDetailPage(cl *vaultapi.Client, name string) Page {
	return newPolicyViewPage(cl, name)
}

func (p *policiesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.resize(msg.w, msg.h)
	case policiesLoadedMsg:
		p.names = msg.names
		p.loaded(msg.err)
	case tea.KeyMsg:
		return p, p.updateKeys(msg)
	}
	return p, nil
}

func (p *policiesPage) Header() string { return "policies" }

func (p *policiesPage) Hints() string {
	return "up/down select  enter view policy  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
