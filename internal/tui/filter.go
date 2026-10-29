package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// filter is the shared incremental-search state for list pages: "/"
// opens it, typing narrows the list live, enter keeps the narrowed
// list, esc clears it. While open, printable keys and enter/esc are
// captured here; navigation keys fall through to the page.
type filter struct {
	active bool
	q      string
}

func (f *filter) empty() bool { return f.q == "" }

// typing reports whether the filter currently captures key input.
func (f *filter) typing() bool { return f.active }

// update consumes keys while the filter is open. It returns true when
// the key was used by the filter and must not reach normal navigation.
func (f *filter) update(msg tea.Msg) bool {
	key, ok := msg.(tea.KeyMsg)
	if !ok || !f.active {
		return false
	}
	switch key.String() {
	case "esc":
		f.active = false
		f.q = ""
	case "enter":
		f.active = false
	case "backspace":
		if f.q != "" {
			f.q = f.q[:len(f.q)-1]
		}
	default:
		s := key.String()
		if len(s) == 1 && s[0] >= 0x20 && s[0] != 0x7f {
			f.q += s
			return true
		}
		return false
	}
	return true
}

// match reports whether s matches the query (case-insensitive).
func (f *filter) match(s string) bool {
	if f.q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(f.q))
}

// view renders the filter input line shown while a query is set (or
// the filter is open).
func (f *filter) view() string {
	if f.q == "" && !f.active {
		return ""
	}
	cursor := ""
	if f.active {
		cursor = "▌"
	}
	return hintKeyStyle.Render("/") + " " + valStyle.Render(f.q) + cursor
}

// highlight styles the query match inside name, rendering the rest in
// the base style.
func (f *filter) highlight(name string, base lipgloss.Style) string {
	if f.q == "" {
		return base.Render(name)
	}
	i := strings.Index(strings.ToLower(name), strings.ToLower(f.q))
	if i < 0 {
		return base.Render(name)
	}
	return base.Render(name[:i]) + selStyle.Render(name[i:i+len(f.q)]) + base.Render(name[i+len(f.q):])
}
