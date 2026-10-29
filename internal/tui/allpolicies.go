package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// allPoliciesMsg delivers every policy's parsed rules.
type allPoliciesMsg struct {
	policies []policyEntry
	warn     []string // policies that did not parse as path rules
	err      error
}

// policyEntry is one policy's parsed path rules.
type policyEntry struct {
	name  string
	paths []policyPath
}

// allPathRow is one policy's grant of one path pattern: one row of the
// union list, where the policy column distinguishes paths granted by
// more than one policy.
type allPathRow struct {
	policy string
	block  policyPath
}

// allPoliciesPage shows every policy's path rules in one list: one
// row per grant (path pattern + granting policy), with fixed
// capability columns; paths granted by several policies appear once
// per policy. The cursor's expansion summarizes the grant; enter
// opens the grant's raw HCL; / filters the list.
type allPoliciesPage struct {
	cl       *vaultapi.Client
	nameList []string // policy names to fetch, fixed at construction
	w, h     int
	filt     filter

	loading  bool
	err      error
	warn     []string
	rows     []allPathRow
	cols     []string
	cur, off int
}

func newAllPoliciesPage(cl *vaultapi.Client, names []string) *allPoliciesPage {
	return &allPoliciesPage{cl: cl, nameList: names, loading: true}
}

func (p *allPoliciesPage) Init() tea.Cmd {
	cl, names := p.cl, p.nameList
	return func() tea.Msg {
		entries := make([]policyEntry, 0, len(names))
		var warn []string
		for _, name := range names {
			raw, err := vault.ReadPolicy(cl, name)
			if err != nil {
				return allPoliciesMsg{err: fmt.Errorf("reading %s: %v", name, err)}
			}
			paths, err := parsePolicyDoc(raw)
			if err != nil {
				// One exotic policy must not sink the whole union; the
				// policy stays reachable through its own view.
				warn = append(warn, name)
				continue
			}
			entries = append(entries, policyEntry{name: name, paths: paths})
		}
		return allPoliciesMsg{policies: entries, warn: warn}
	}
}

// visible returns the grants matching the filter (by path or policy).
func (p *allPoliciesPage) visible() []allPathRow {
	if p.filt.empty() {
		return p.rows
	}
	var out []allPathRow
	for _, r := range p.rows {
		if p.filt.match(r.block.path) || p.filt.match(r.policy) {
			out = append(out, r)
		}
	}
	return out
}

func (p *allPoliciesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case resizeMsg:
		p.w, p.h = msg.w, msg.h
	case allPoliciesMsg:
		p.loading = false
		p.err = msg.err
		p.warn = msg.warn
		p.rows, p.cols = flattenAllPaths(msg.policies)
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		if p.loading || p.err != nil {
			return p, nil
		}
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
				// the grant's raw HCL with its lead comment, the
				// ground truth behind the expansion's summary
				g := vis[p.cur]
				src, path, policy := g.block.src, g.block.path, g.policy
				if g.block.lead != "" {
					src = g.block.lead + "\n" + src
				}
				return p, Push(newDetailPage(
					fmt.Sprintf("path: %s (%s)", path, policy),
					func() (string, []pageLink, error) { return src, nil, nil },
				))
			}
		}
	}
	return p, nil
}

// flattenAllPaths turns every policy's path rules into one sorted
// list of grants (path, then policy), plus the fixed capability
// columns. Paths granted by several policies appear once per policy.
func flattenAllPaths(entries []policyEntry) ([]allPathRow, []string) {
	var rows []allPathRow
	var blocks []policyPath
	for _, e := range entries {
		for _, pp := range e.paths {
			rows = append(rows, allPathRow{policy: e.name, block: pp})
			blocks = append(blocks, pp)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].block.path != rows[j].block.path {
			return rows[i].block.path < rows[j].block.path
		}
		return rows[i].policy < rows[j].policy
	})
	return rows, policyCols(blocks)
}

func (p *allPoliciesPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading all policies ...")
	}
	if p.err != nil {
		return errStyle.Render("error: " + p.err.Error())
	}
	vis := p.visible()
	if len(vis) == 0 {
		if p.filt.empty() && len(p.rows) == 0 {
			return dimStyle.Render("(no path rules in any policy)")
		}
		return p.filt.view() + "\n\n" + dimStyle.Render("(no matches)")
	}
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	p.cur, p.off = window(p.cur, p.off, len(vis), rows)

	longest := 0
	for _, r := range vis {
		if len(r.block.path) > longest {
			longest = len(r.block.path)
		}
	}
	colsW := len(p.cols)*2 - 1
	pathW := min(longest, max(p.w-colsW-30, 10))

	var b strings.Builder
	b.WriteString("  " + dimStyle.Render("PATH") + listInfo(p.cur, len(vis), rows, "paths") + "\n")
	if len(p.warn) > 0 {
		b.WriteString("  " + dimStyle.Render("(unparseable, not shown: "+strings.Join(p.warn, ", ")+")") + "\n")
	}
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		r := vis[i]
		path := fmt.Sprintf("%-*s", pathW, truncate(r.block.path, pathW))
		cols := capColumns(r.block.caps, p.cols)
		polW := max(p.w-2-pathW-2-colsW-3, 0)
		pols := truncate(r.policy, polW)
		if i == p.cur {
			b.WriteString(selBar(p.w, "  "+path+"  "+cols+"  "+pols) + "\n")
			continue
		}
		b.WriteString("  " + itemStyle.Render(path) + "  " + dimStyle.Render(cols) + "  " + dimStyle.Render(pols) + "\n")
	}

	b.WriteString("\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// expansion renders the selected grant's summary: the policy name,
// its caps and the lead comment. The full HCL is a keystroke away
// (enter) rather than duplicated here.
func (p *allPoliciesPage) expansion(vis []allPathRow) []string {
	if len(vis) == 0 || p.cur >= len(vis) {
		return nil
	}
	g := vis[p.cur]
	var lines []string
	lines = append(lines, selStyle.Render(g.policy)+"  "+dimStyle.Render(strings.Join(sortCaps(g.block.caps), ", ")))
	for _, c := range g.block.comments {
		lines = append(lines, dimStyle.Render(c))
	}
	return lines
}

func (p *allPoliciesPage) Header() string { return "policies / all" }

func (p *allPoliciesPage) Typing() bool { return p.filt.typing() }

func (p *allPoliciesPage) Loading() bool { return p.loading }

func (p *allPoliciesPage) Hints() string {
	return "up/down select  enter hcl  / filter  ctrl+l redraw  q quit"
}
