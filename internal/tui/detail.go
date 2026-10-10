package tui

import (
	"sync/atomic"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/reflow/wordwrap"
)

var detailSeq atomic.Int64

type detailLoadedMsg struct {
	id      int64
	content string
	err     error
}

// detailPage shows a scrollable, wrapped text document (a secret, a role
// config, a policy) loaded asynchronously by its loader function.
type detailPage struct {
	id      int64
	title   string
	loader  func() (string, error)
	loading bool
	err     error
	content string
	vp      viewport.Model
	w, h    int
}

func newDetailPage(title string, loader func() (string, error)) *detailPage {
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
		content, err := loader()
		return detailLoadedMsg{id: id, content: content, err: err}
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
		p.layout()
	case tea.KeyMsg:
		// Let the viewport handle scrolling keys (arrows, pgup/pgdn,
		// home/end, u/d, g/G).
		p.vp, _ = p.vp.Update(msg)
	}
	return p, nil
}

func (p *detailPage) layout() {
	p.vp.Width = p.w
	p.vp.Height = p.h
	p.vp.SetContent(wrapText(p.content, p.w))
}

func (p *detailPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	return p.vp.View()
}

func (p *detailPage) Header() string { return p.title }

func (p *detailPage) Hints() string {
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
