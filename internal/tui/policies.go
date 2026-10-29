package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

type policiesPage struct {
	cl      *vaultapi.Client
	names   []string
	filt    filter
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

// allPoliciesEntry is the synthetic first row of the policies list:
// it opens the union of every policy's paths.
const allPoliciesEntry = "All policies (union)"

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

// newPolicyDetailPage builds the structured viewer for one policy's
// rules (path list with expansion; h shows the raw document).
func newPolicyDetailPage(cl *vaultapi.Client, name string) Page {
	return newPolicyViewPage(cl, name)
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
			p.cur = pageJump(p.cur, max(p.h-3, 1), len(vis), false)
		case "pgdown":
			p.cur = pageJump(p.cur, max(p.h-3, 1), len(vis), true)
		case "enter":
			if p.cur < len(vis) {
				if vis[p.cur] == allPoliciesEntry {
					return p, Push(newAllPoliciesPage(p.cl, p.names))
				}
				return p, Push(newPolicyDetailPage(p.cl, vis[p.cur]))
			}
		case "r":
			p.loading = true
			p.err = nil
			return p, p.Init()
		}
	}
	return p, nil
}

// visible returns the union row (when it matches the filter) plus the
// policy names matching the filter.
func (p *policiesPage) visible() []string {
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
	b.WriteString("  " + dimStyle.Render("ACL POLICIES") + listInfo(p.cur, len(vis), rows, "policies") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+vis[i]) + "\n")
			continue
		}
		if vis[i] == allPoliciesEntry {
			b.WriteString("  " + typeStyle.Render(vis[i]) + "\n")
			continue
		}
		b.WriteString("  " + p.filt.highlight(vis[i], itemStyle) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (p *policiesPage) Header() string { return "policies" }

func (p *policiesPage) Typing() bool { return p.filt.typing() }

func (p *policiesPage) Loading() bool { return p.loading }

func (p *policiesPage) Hints() string {
	return "up/down select  enter view policy  / filter  r refresh  esc back  ctrl+l redraw  q quit"
}
