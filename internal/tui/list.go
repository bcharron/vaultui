package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// listPage is the shared skeleton of the app's filterable list pages
// (secret engines, auth methods, policies, the KV browser, leases, KV
// versions): a "/" filter narrowing the list live, cursor navigation
// with pgup/pgdown, an optional refresh key, and the standard
// loading/error/empty rendering. A page embeds it and supplies the
// items, the row rendering and the enter/refresh actions as function
// fields; the embedded View, Typing and Loading then cover those parts
// of Page.
type listPage[T any] struct {
	filt     filter
	cur, off int
	loading  bool
	err      error
	w, h     int

	// items is the full, unfiltered list (for the empty check);
	// visible is the filter-narrowed list.
	items   func() []T
	visible func() []T
	// row renders one list row without the trailing newline; sel asks
	// for the highlighted selection bar.
	row func(item T, sel bool) string
	// onEnter opens the selected item; nil disables enter.
	onEnter func(item T) tea.Cmd
	// reload re-fetches the list for the "r" key; nil disables it.
	reload func() tea.Cmd
	// onKey handles page-specific extra keys (e.g. "v" for version
	// history); it reports whether the key was consumed.
	onKey func(key string) (tea.Cmd, bool)

	// top renders an optional line above everything (the KV browser's
	// path); nil for none.
	top func() string
	// title is the styled header line above the list, with the item
	// count appended (listInfo); empty for none.
	title string
	// noun names the items in the count ("mounts", "keys", ...).
	noun string
	// headLines is the number of lines above the list rows: title and
	// a blank line (2), or top line, count and a blank line (3).
	headLines   int
	loadingText string
	emptyText   string
	errPrefix   string
}

// resize records the content-area dimensions.
func (l *listPage[T]) resize(w, h int) { l.w, l.h = w, h }

// loaded records a data message's outcome and resets the cursor.
func (l *listPage[T]) loaded(err error) {
	l.loading = false
	l.err = err
	l.cur, l.off = 0, 0
}

// Typing reports whether the filter captures key input.
func (l *listPage[T]) Typing() bool { return l.filt.typing() }

// Loading reports whether the page is waiting on its data.
func (l *listPage[T]) Loading() bool { return l.loading }

// rows is the number of list rows that fit the content area.
func (l *listPage[T]) rows() int {
	rows := l.h - l.headLines
	if l.filt.view() != "" {
		rows--
	}
	return max(rows, 1)
}

// updateKeys handles the shared list keys: the filter, cursor movement,
// enter and refresh. Navigation is expressed as a command (Push,
// reload), so the page itself never changes.
func (l *listPage[T]) updateKeys(msg tea.KeyMsg) tea.Cmd {
	if l.filt.typing() {
		if l.filt.update(msg) {
			l.cur, l.off = 0, 0
			return nil
		}
	} else if msg.String() == "/" {
		l.filt.active = true
		return nil
	}
	vis := l.visible()
	switch msg.String() {
	case "up", "k":
		if l.cur > 0 {
			l.cur--
		}
	case "down", "j":
		if l.cur < len(vis)-1 {
			l.cur++
		}
	case "pgup":
		l.cur = pageJump(l.cur, l.rows(), len(vis), false)
	case "pgdown":
		l.cur = pageJump(l.cur, l.rows(), len(vis), true)
	case "enter":
		if l.onEnter != nil && l.cur < len(vis) {
			return l.onEnter(vis[l.cur])
		}
	case "r":
		if l.reload != nil {
			l.loading = true
			l.err = nil
			return l.reload()
		}
	default:
		if l.onKey != nil {
			if cmd, ok := l.onKey(msg.String()); ok {
				return cmd
			}
		}
	}
	return nil
}

// View renders the standard list page: the optional top line, then the
// loading/error/empty states, then the title with the item count, the
// filter line, and the windowed rows.
func (l *listPage[T]) View() string {
	var b strings.Builder
	if l.top != nil {
		b.WriteString(l.top())
		b.WriteString("\n")
	}
	if l.loading {
		b.WriteString(loadingStyle.Render(l.loadingText))
		return b.String()
	}
	if l.err != nil {
		b.WriteString(errStyle.Render(l.errPrefix + l.err.Error()))
		return b.String()
	}
	if len(l.items()) == 0 {
		b.WriteString(dimStyle.Render(l.emptyText))
		return b.String()
	}
	vis := l.visible()
	if len(vis) == 0 {
		b.WriteString(l.filt.view() + "\n\n")
		b.WriteString(dimStyle.Render("(no matches)"))
		return b.String()
	}
	rows := l.rows()
	l.cur, l.off = window(l.cur, l.off, len(vis), rows)
	b.WriteString(l.title + listInfo(l.cur, len(vis), rows, l.noun) + "\n")
	if fl := l.filt.view(); fl != "" {
		b.WriteString(fl + "\n")
	}
	b.WriteString("\n")
	for i := l.off; i < min(l.off+rows, len(vis)); i++ {
		b.WriteString(l.row(vis[i], i == l.cur) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// window(cur, off, n, rows) scrolls a fixed-size window over n items
// keeping cur visible.
func window(cur, off, n, rows int) (int, int) {
	if rows < 1 {
		rows = 1
	}
	if cur < off {
		off = cur
	}
	if cur >= off+rows {
		off = cur - rows + 1
	}
	if off > n-rows {
		off = n - rows
	}
	if off < 0 {
		off = 0
	}
	return cur, off
}

// pageJump moves cur by one page of rows, clamped to [0, n-1]; the
// pgup/pgdown navigation of the list pages.
func pageJump(cur, rows, n int, down bool) int {
	if rows < 1 {
		rows = 1
	}
	if n < 1 {
		return 0
	}
	if down {
		return min(cur+rows, n-1)
	}
	return max(cur-rows, 0)
}
