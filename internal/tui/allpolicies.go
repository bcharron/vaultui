package tui

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// allPoliciesMsg delivers every readable policy's parsed rules.
type allPoliciesMsg struct {
	policies []policyEntry
	warn     []string       // policies that did not parse as path rules
	failed   []failedPolicy // policies that could not be read
}

// policyEntry is one policy's parsed path rules.
type policyEntry struct {
	name  string
	paths []policyPath
}

// policyReadError describes a failed policy read: reason is the
// single line shown in the union row; the request details feed the
// row's expansion pane.
type policyReadError struct {
	reason  string   // one line, for the row
	method  string   // HTTP method of the failed request
	apiPath string   // request path, e.g. sys/policies/acl/topsecret
	status  int      // HTTP status, 0 when the request never got one
	detail  []string // the server's error lines, for the expansion
}

// failedPolicy is a policy the token could not read (e.g. permission
// denied). It still gets a row in the union list — rendered in red —
// so the failure is visible without a note line wrapping over the
// content; the row's expansion shows the request details.
type failedPolicy struct {
	name string
	fail policyReadError
}

// allPathRow is one policy's grant of one path pattern: one row of the
// union list, where the policy column distinguishes paths granted by
// more than one policy. A row with fail set is an unreadable policy,
// not a grant.
type allPathRow struct {
	policy string
	block  policyPath
	fail   *policyReadError // non-nil: the policy could not be read
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
		var failed []failedPolicy
		for _, name := range names {
			raw, err := vault.ReadPolicy(cl, name)
			if err != nil {
				// One unreadable policy (e.g. permission denied) must
				// not sink the whole union; it gets a red row below
				// and the rest still load.
				failed = append(failed, failedPolicy{name: name, fail: describeReadError(err)})
				continue
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
		return allPoliciesMsg{policies: entries, warn: warn, failed: failed}
	}
}

// policyReadReason is the one-line form of a failed policy read.
func policyReadReason(err error) string {
	return describeReadError(err).reason
}

// describeReadError captures a failed policy read: a single-line
// reason for the union row (Vault wraps denials in a multierror whose
// bullets are the reason), plus the request details for the row's
// expansion.
func describeReadError(err error) policyReadError {
	var re *vaultapi.ResponseError
	if errors.As(err, &re) {
		e := policyReadError{
			method: re.HTTPMethod,
			status: re.StatusCode,
			detail: re.Errors,
		}
		if u, err := url.Parse(re.URL); err == nil {
			e.apiPath = strings.TrimPrefix(u.Path, "/v1/")
		}
		e.reason = condenseErrLines(strings.Join(re.Errors, "\n"))
		if e.reason == "" {
			e.reason = fmt.Sprintf("status %d", re.StatusCode)
		}
		return e
	}
	msg := err.Error()
	return policyReadError{
		reason: condenseErrLines(msg),
		detail: strings.Split(msg, "\n"),
	}
}

// condenseErrLines reduces an error text to a single line: the
// multierror bullets when present ("* permission denied"), else the
// whole message with the whitespace collapsed.
func condenseErrLines(s string) string {
	var bullets []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "* ") {
			bullets = append(bullets, strings.TrimPrefix(l, "* "))
		}
	}
	if len(bullets) > 0 {
		return strings.Join(bullets, "; ")
	}
	return strings.Join(strings.Fields(s), " ")
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
		p.warn = msg.warn
		p.rows, p.cols = flattenAllPaths(msg.policies, msg.failed)
		p.cur, p.off = 0, 0
	case tea.KeyMsg:
		if p.loading {
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
			p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), false)
		case "pgdown":
			p.cur = pageJump(p.cur, p.rowsVisible(), len(vis), true)
		case "enter":
			if p.cur < len(vis) {
				g := vis[p.cur]
				if g.fail != nil {
					// an unreadable policy has no HCL; re-attempt the
					// read so the error (or a fixed permission) shows
					// on the detail page
					cl, name := p.cl, g.policy
					return p, Push(newDetailPage(
						"policy: "+name,
						func() (string, []pageLink, error) {
							raw, err := vault.ReadPolicy(cl, name)
							if err != nil {
								return "", nil, errors.New(policyReadReason(err))
							}
							return raw, nil, nil
						},
					))
				}
				// the grant's raw HCL with its lead comment, the
				// ground truth behind the expansion's summary
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
// Unreadable policies trail the list as error rows, sorted by name.
func flattenAllPaths(entries []policyEntry, failed []failedPolicy) ([]allPathRow, []string) {
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
	sort.Slice(failed, func(i, j int) bool { return failed[i].name < failed[j].name })
	for _, f := range failed {
		fail := f.fail
		rows = append(rows, allPathRow{policy: f.name, fail: &fail})
	}
	return rows, policyCols(blocks)
}

// rowsVisible is the number of list rows the view shows: the top half
// of the content area, less the filter line.
func (p *allPoliciesPage) rowsVisible() int {
	rows := max(p.h/2, 1)
	if p.filt.view() != "" {
		rows = max(rows-1, 1)
	}
	return rows
}

func (p *allPoliciesPage) View() string {
	if p.loading {
		return loadingStyle.Render("loading all policies ...")
	}
	vis := p.visible()
	if len(vis) == 0 {
		var b strings.Builder
		for _, n := range p.noteLines() {
			b.WriteString("  " + dimStyle.Render(n) + "\n")
		}
		if p.filt.empty() && len(p.rows) == 0 {
			b.WriteString(dimStyle.Render("(no path rules in any policy)"))
		} else {
			b.WriteString(p.filt.view() + "\n\n" + dimStyle.Render("(no matches)"))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	rows := p.rowsVisible()
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
	for _, n := range p.noteLines() {
		b.WriteString("  " + dimStyle.Render(n) + "\n")
	}
	if fl := p.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := p.off; i < min(p.off+rows, len(vis)); i++ {
		r := vis[i]
		if r.fail != nil {
			// unreadable policy: one red line, no note piling up
			// above the list
			line := truncate("(unreadable) "+r.policy+": "+r.fail.reason, max(p.w-2, 1))
			if i == p.cur {
				b.WriteString(selBar(p.w, "  "+line) + "\n")
			} else {
				b.WriteString("  " + errStyle.Render(line) + "\n")
			}
			continue
		}
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

	b.WriteString(hRule(p.w) + "\n")
	for _, l := range p.expansion(vis) {
		b.WriteString(l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// noteLines lists the policies left out of the union entirely
// (unparseable ones; unreadable ones have their own rows).
func (p *allPoliciesPage) noteLines() []string {
	if len(p.warn) > 0 {
		return []string{"(unparseable, not shown: " + strings.Join(p.warn, ", ") + ")"}
	}
	return nil
}

// expansion renders the selected grant's summary: the policy name,
// its caps and the lead comment. The full HCL is a keystroke away
// (enter) rather than duplicated here. An unreadable policy's row
// expands to the read error's details instead.
func (p *allPoliciesPage) expansion(vis []allPathRow) []string {
	if len(vis) == 0 || p.cur >= len(vis) {
		return nil
	}
	g := vis[p.cur]
	if g.fail != nil {
		return p.errExpansion(g.policy, g.fail)
	}
	var lines []string
	lines = append(lines, selStyle.Render(g.policy)+"  "+dimStyle.Render(strings.Join(sortCaps(g.block.caps), ", ")), "")
	for _, c := range g.block.comments {
		lines = append(lines, dimStyle.Render(c))
	}
	return lines
}

// errExpansion renders the bottom pane for an unreadable policy: the
// one-line reason, then the request that failed and the server's
// error lines.
func (p *allPoliciesPage) errExpansion(policy string, fail *policyReadError) []string {
	lines := []string{errStyle.Render(policy + ": " + fail.reason), ""}
	if fail.apiPath != "" {
		lines = append(lines,
			keyStyle.Render("request: ")+valStyle.Render(fail.method+" "+fail.apiPath),
			keyStyle.Render("status: ")+valStyle.Render(fmt.Sprintf("%d", fail.status)),
		)
	}
	var detail []string
	for _, d := range fail.detail {
		for _, l := range strings.Split(d, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				detail = append(detail, l)
			}
		}
	}
	if len(detail) > 0 {
		lines = append(lines, keyStyle.Render("errors:"))
		for _, d := range detail {
			lines = append(lines, "  "+dimStyle.Render(d))
		}
	}
	return lines
}

func (p *allPoliciesPage) Header() string { return "policies / all" }

func (p *allPoliciesPage) Typing() bool { return p.filt.typing() }

func (p *allPoliciesPage) Loading() bool { return p.loading }

func (p *allPoliciesPage) Hints() string {
	return "up/down select  enter hcl  / filter  ctrl+l redraw  q quit"
}
