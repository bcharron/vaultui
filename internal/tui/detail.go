package tui

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/muesli/reflow/wordwrap"
)

var detailSeq atomic.Int64

type detailLoadedMsg struct {
	id      int64
	content string
	links   []pageLink
	err     error
}

// pageLink is a navigable cross-reference inside a detail page. Pressing
// the number shown next to it pushes page() onto the stack.
type pageLink struct {
	label string
	page  func() Page
}

// policyLinks builds page links that jump to policy documents.
func policyLinks(cl *vaultapi.Client, pols []string) []pageLink {
	links := make([]pageLink, 0, len(pols))
	for _, pol := range pols {
		name := pol
		links = append(links, pageLink{
			label: name,
			page:  func() Page { return newPolicyDetailPage(cl, name) },
		})
	}
	return links
}

// detailPage shows a scrollable, wrapped text document (a secret, a role
// config, a policy) loaded asynchronously by its loader function, with
// optional numbered cross-reference links.
type detailPage struct {
	id        int64
	title     string
	loader    func() (string, []pageLink, error)
	linkTitle string
	loading   bool
	err       error
	content   string
	links     []pageLink
	vp        viewport.Model
	w, h      int
}

func newDetailPage(title string, loader func() (string, []pageLink, error)) *detailPage {
	return &detailPage{
		id:      detailSeq.Add(1),
		title:   title,
		loader:  loader,
		loading: true,
		vp:      viewport.New(0, 0),
	}
}

func (p *detailPage) Init() tea.Cmd {
	id, loader := p.id, p.loader
	return func() tea.Msg {
		content, links, err := loader()
		return detailLoadedMsg{id: id, content: content, links: links, err: err}
	}
}

func (p *detailPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
		p.layout()
	case detailLoadedMsg:
		if msg.id != p.id {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.content = msg.content
		p.links = msg.links
		p.layout()
	case tea.KeyMsg:
		// Number keys open the corresponding link (e.g. a linked
		// policy); everything else scrolls the viewport.
		if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if d := int(s[0] - '0'); d <= len(p.links) {
				return p, Push(p.links[d-1].page())
			}
		}
		p.vp, _ = p.vp.Update(msg)
	}
	return p, nil
}

func (p *detailPage) layout() {
	p.vp.Width = p.w
	h := p.h - p.linksHeight()
	if h < 1 {
		h = 1
	}
	p.vp.Height = h
	p.vp.SetContent(wrapText(p.content, p.w))
}

// linksHeight is the vertical space the links section occupies: one line
// plus a blank separator, or zero.
func (p *detailPage) linksHeight() int {
	if len(p.links) == 0 {
		return 0
	}
	return 2
}

func (p *detailPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	var b strings.Builder
	if len(p.links) > 0 {
		title := p.linkTitle
		if title == "" {
			title = "links"
		}
		b.WriteString(dimStyle.Render(title + ": "))
		items := make([]string, 0, len(p.links))
		for i, l := range p.links {
			items = append(items, fmt.Sprintf("%s %s",
				dimStyle.Render(fmt.Sprintf("[%d]", i+1)), selStyle.Render(l.label)))
		}
		b.WriteString(strings.Join(items, "  "))
		b.WriteString("\n\n")
	}
	b.WriteString(p.vp.View())
	return b.String()
}

func (p *detailPage) Header() string { return p.title }

func (p *detailPage) Typing() bool { return false }

func (p *detailPage) Loading() bool { return p.loading }

func (p *detailPage) Hints() string {
	if len(p.links) > 0 {
		return "1-9 open link  up/down scroll  esc back  q quit"
	}
	return "up/down scroll  esc back  q quit"
}

func wrapText(s string, w int) string {
	if s == "" {
		return ""
	}
	if w < 20 {
		w = 20
	}
	return wordwrap.String(s, w)
}
