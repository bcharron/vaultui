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

// pathGrant is one policy's grant of one path pattern.
type pathGrant struct {
	policy string
	block  policyPath
}

// allPath is one unique path pattern with every policy granting it.
type allPath struct {
	path   string
	grants []pathGrant
}

// allPoliciesPage shows the union of every policy's path rules: one
// row per unique path pattern with fixed capability columns and the
// granting policies, and the cursor's expansion showing each grant in
// detail. enter opens the first granting policy with the path preset
// as its filter; / filters the path list.
type allPoliciesPage struct {
	cl       *vaultapi.Client
	nameList []string // policy names to fetch, fixed at construction
	w, h     int
	filt     filter

	loading  bool
	err      error
	warn     []string
	paths    []allPath
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

// visible returns the paths matching the filter.
func (p *allPoliciesPage) visible() []allPath {
	if p.filt.empty() {
		return p.paths
	}
	var out []allPath
	for _, ap := range p.paths {
		if p.filt.match(ap.path) {
			out = append(out, ap)
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
		p.paths, p.cols = mergeAllPaths(msg.policies)
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
				np := newPolicyViewPage(p.cl, vis[p.cur].grants[0].policy)
				np.filt.q = vis[p.cur].path
				return p, Push(np)
			}
		}
	}
	return p, nil
}

// mergeAllPaths unions every policy's path rules into one sorted list
// of unique paths, each carrying its granting policies sorted by
// name, plus the fixed capability columns.
func mergeAllPaths(entries []policyEntry) ([]allPath, []string) {
	byPath := map[string][]pathGrant{}
	var blocks []policyPath
	for _, e := range entries {
		for _, pp := range e.paths {
			byPath[pp.path] = append(byPath[pp.path], pathGrant{policy: e.name, block: pp})
			blocks = append(blocks, pp)
		}
	}
	paths := make([]allPath, 0, len(byPath))
	for path, grants := range byPath {
		sort.Slice(grants, func(i, j int) bool { return grants[i].policy < grants[j].policy })
		paths = append(paths, allPath{path: path, grants: grants})
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].path < paths[j].path })
	return paths, policyCols(blocks)
}

// unionCaps returns the capabilities granted to a path across all its
// granting policies.
func (a allPath) unionCaps() []string {
	var out []string
	for _, g := range a.grants {
		out = append(out, g.block.caps...)
	}
	return out
}

// policies returns the names of the granting policies, sorted.
func (a allPath) policies() []string {
	out := make([]string, 0, len(a.grants))
	for _, g := range a.grants {
		out = append(out, g.policy)
	}
	return out
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
		if p.filt.empty() && len(p.paths) == 0 {
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
	for _, ap := range vis {
		if len(ap.path) > longest {
			longest = len(ap.path)
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
		ap := vis[i]
		path := fmt.Sprintf("%-*s", pathW, truncate(ap.path, pathW))
		cols := capColumns(ap.unionCaps(), p.cols)
		pols := strings.Join(ap.policies(), ",")
		polW := max(p.w-2-pathW-2-colsW-3, 0)
		pols = truncate(pols, polW)
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

// expansion renders the selected path's grants: one section per
// granting policy with its comment, capabilities, attributes and HCL
// block.
func (p *allPoliciesPage) expansion(vis []allPath) []string {
	if len(vis) == 0 || p.cur >= len(vis) {
		return nil
	}
	ap := vis[p.cur]
	var lines []string
	for i, g := range ap.grants {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, selStyle.Render(g.policy)+"  "+dimStyle.Render(strings.Join(sortCaps(g.block.caps), ", ")))
		for _, c := range g.block.comments {
			lines = append(lines, dimStyle.Render(c))
		}
		if g.block.src != "" {
			for _, l := range strings.Split(g.block.src, "\n") {
				lines = append(lines, "  "+valStyle.Render(l))
			}
		}
	}
	area := max(p.h-p.h/2-4, 4)
	if len(lines) > area {
		lines = append(lines[:area-1], dimStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-area+1)))
	}
	return lines
}

func (p *allPoliciesPage) Header() string { return "policies / all" }

func (p *allPoliciesPage) Typing() bool { return p.filt.typing() }

func (p *allPoliciesPage) Loading() bool { return p.loading }

func (p *allPoliciesPage) Hints() string {
	return "up/down select  enter open policy  / filter  ctrl+l redraw  q quit"
}
