package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// policyDocMsg delivers one policy's raw rules.
type policyDocMsg struct {
	name string
	raw  string
	err  error
}

// policyViewPage shows one ACL policy as an alphabetical list of path
// rules, one fixed column per capability so grants line up
// vertically; the cursor's rule expands at the bottom with its
// comments, capabilities, attributes and verbatim HCL block. / filters
// the path list, h toggles the raw HCL document, which is also the
// fallback for policies that do not parse as plain path rules.
type policyViewPage struct {
	cl   *vaultapi.Client
	name string
	w, h int
	filt filter

	loading bool
	err     error
	raw     string
	paths   []policyPath
	cols    []string // capability columns, fixed across rows
	parseOK bool
	rawMode bool

	cur, off int
	vp       viewport.Model
}

func newPolicyViewPage(cl *vaultapi.Client, name string) *policyViewPage {
	return &policyViewPage{cl: cl, name: name, loading: true, parseOK: true, vp: viewport.New(0, 0)}
}

func (p *policyViewPage) Init() tea.Cmd {
	cl, name := p.cl, p.name
	return func() tea.Msg {
		raw, err := vault.ReadPolicy(cl, name)
		return policyDocMsg{name: name, raw: raw, err: err}
	}
}

// rowsVisible is the number of list rows the view shows.
func (p *policyViewPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

// visible returns the paths matching the filter.
func (p *policyViewPage) visible() []policyPath {
	if p.filt.empty() {
		return p.paths
	}
	var out []policyPath
	for _, pp := range p.paths {
		if p.filt.match(pp.path) {
			out = append(out, pp)
		}
	}
	return out
}

// structured reports whether the path-list view applies (as opposed
// to the raw document view).
func (p *policyViewPage) structured() bool {
	return p.parseOK && len(p.paths) > 0 && !p.rawMode
}

func (p *policyViewPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
		p.layout()
	case policyDocMsg:
		if msg.name != p.name {
			return p, nil
		}
		p.loading = false
		p.err = msg.err
		p.raw = msg.raw
		p.cur, p.off = 0, 0
		if msg.err == nil {
			if paths, err := parsePolicyDoc(msg.raw); err == nil {
				p.paths, p.cols, p.parseOK = paths, policyCols(paths), true
			} else {
				p.paths, p.cols, p.parseOK = nil, nil, false
			}
		}
		p.layout()
	case tea.KeyMsg:
		if p.loading || p.err != nil {
			return p, nil
		}
		if p.filt.typing() {
			if p.filt.update(msg) {
				p.cur, p.off = 0, 0
				return p, nil
			}
		} else if msg.String() == "/" && p.structured() {
			p.filt.active = true
			return p, nil
		}
		switch msg.String() {
		case "h":
			if p.parseOK && strings.TrimSpace(p.raw) != "" {
				p.rawMode = !p.rawMode
				p.layout()
				return p, nil
			}
		}
		if !p.structured() {
			p.vp, _ = p.vp.Update(msg)
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
			p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), false)
		case "pgdown":
			p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), true)
		}
	}
	return p, nil
}

// layout recomputes the raw viewport; it is shown for the raw toggle
// and whenever the document did not parse as path rules.
func (p *policyViewPage) layout() {
	p.vp.Width = p.w
	p.vp.Height = p.h
	content := p.raw
	if !p.parseOK && p.err == nil {
		content = dimStyle.Render("(not parseable as path rules; showing the raw policy)") + "\n\n" + p.raw
	}
	p.vp.SetContent(wrapText(content, p.w))
}

func (p *policyViewPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading policy ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	if p.structured() {
		return p.listView()
	}
	if !p.rawMode && len(p.paths) == 0 && p.parseOK {
		if p.name == "root" {
			return dimStyle.Render("root policy: implicit full access (no path rules)")
		}
		return dimStyle.Render("(no path rules)")
	}
	return p.vp.View()
}

// listView renders the alphabetical path list with fixed capability
// columns and the cursor's rule expanded at the bottom.
func (p *policyViewPage) listView() string {
	vis := p.visible()
	if len(vis) == 0 {
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := p.rowsVisible()
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	longest := 0
	for _, pp := range vis {
		if len(pp.path) > longest {
			longest = len(pp.path)
		}
	}
	colsW := len(p.cols)*2 - 1
	pathW := min(longest, max(p.w-2-colsW-4, 10))

	var b strings.Builder
	b.WriteString("  " + dimStyle.Render("PATH") + listInfo(p.cur, len(vis), rows, "paths") + "\n")
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		pp := vis[i]
		path := fmt.Sprintf("%-*s", pathW, truncate(pp.path, pathW))
		cols := capColumns(pp.caps, p.cols)
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+path+"  "+cols) + "\n")
			continue
		}
		b.WriteString("  " + itemStyle.Render(path) + "  " + dimStyle.Render(cols) + "\n")
	}

	b.WriteString("\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// expansion renders the selected rule's detail block: path title,
// lead comments, capabilities, other attributes, and the verbatim
// HCL block.
func (p *policyViewPage) expansion(vis []policyPath) []string {
	if len(vis) == 0 || p.cur >= len(vis) {
		return nil
	}
	pp := vis[p.cur]
	lines := []string{titleStyle.Render(pp.path)}
	for _, c := range pp.comments {
		lines = append(lines, dimStyle.Render(c))
	}
	if len(pp.caps) > 0 {
		lines = append(lines, keyStyle.Render("capabilities: ")+valStyle.Render(strings.Join(sortCaps(pp.caps), ", ")))
	} else {
		lines = append(lines, keyStyle.Render("capabilities: ")+dimStyle.Render("(none)"))
	}
	for _, a := range pp.attrs {
		if len(a.values) == 1 {
			lines = append(lines, keyStyle.Render(a.name+": ")+valStyle.Render(a.values[0]))
			continue
		}
		lines = append(lines, keyStyle.Render(a.name+":"))
		for _, v := range a.values {
			lines = append(lines, "  "+valStyle.Render(v))
		}
	}
	if pp.src != "" {
		lines = append(lines, dimStyle.Render("-- hcl --"))
		for _, l := range strings.Split(pp.src, "\n") {
			lines = append(lines, "  "+valStyle.Render(l))
		}
	}

	// The expansion shares the content area with the list; when the
	// selected rule is too rich, elide the tail rather than push the
	// layout around.
	area := max(p.h-p.h/2-4, 4)
	if len(lines) > area {
		lines = append(lines[:area-1], dimStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-area+1)))
	}
	return lines
}

func (p *policyViewPage) Header() string { return "policy: " + p.name }

func (p *policyViewPage) Typing() bool { return p.filt.typing() }

func (p *policyViewPage) Loading() bool { return p.loading }

func (p *policyViewPage) Hints() string {
	return "up/down select  / filter  h raw view  esc back  q quit"
}
