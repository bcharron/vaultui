package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	vaultapi "github.com/hashicorp/vault/api"
)

// resizeMsg carries the content-area dimensions (width x height) to every
// page whenever the terminal is resized.
type resizeMsg struct{ w, h int }

// pushMsg/popMsg drive the navigation stack.
type pushMsg struct{ page Page }
type popMsg struct{}

// Push returns a command that navigates onto a new page.
func Push(p Page) tea.Cmd { return func() tea.Msg { return pushMsg{p} } }

// Pop returns a command that navigates back one page.
func Pop() tea.Cmd { return func() tea.Msg { return popMsg{} } }

// Page is one screen of the TUI. Update returns the (possibly replaced)
// page, mirroring tea.Model semantics. Typing reports whether the page
// currently captures arbitrary key input (a text field); while true the
// app does not intercept q/esc.
type Page interface {
	Init() tea.Cmd
	Update(tea.Msg) (Page, tea.Cmd)
	View() string
	Header() string
	Hints() string
	Typing() bool
}

// App is the root model: it owns the page stack and the global chrome
// (header with address, footer with key hints).
type App struct {
	cl    *vaultapi.Client
	addr  string
	token string
	ns    string
	stack []Page
	w, h  int
}

// New builds the app around an authenticated Vault client.
func New(cl *vaultapi.Client) App {
	return App{
		cl:    cl,
		addr:  cl.Address(),
		token: maskToken(cl.Token()),
		ns:    cl.Namespace(),
		stack: []Page{newHomePage(cl)},
	}
}

func (a App) top() Page { return a.stack[len(a.stack)-1] }

func (a App) Init() tea.Cmd { return a.top().Init() }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		return a.broadcast(a.dims())

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "q":
			if !a.top().Typing() {
				return a, tea.Quit
			}
		case "esc":
			if !a.top().Typing() {
				if len(a.stack) > 1 {
					a.stack = a.stack[:len(a.stack)-1]
					return a, nil
				}
				return a, tea.Quit
			}
		}
		var cmd tea.Cmd
		a.stack[len(a.stack)-1], cmd = a.stack[len(a.stack)-1].Update(msg)
		return a, cmd

	case pushMsg:
		a.stack = append(a.stack, msg.page)
		var cmd tea.Cmd
		msg.page, cmd = msg.page.Update(a.dims())
		return a, tea.Batch(cmd, msg.page.Init())

	case popMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		return a, nil

	default:
		// Data messages go to every page in the stack so that pages
		// buried under others still receive their async results.
		return a.broadcast(msg)
	}
}

func (a App) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, len(a.stack))
	for i := range a.stack {
		a.stack[i], cmds[i] = a.stack[i].Update(msg)
	}
	return a, tea.Batch(cmds...)
}

// dims returns the current content-area dimensions: the terminal minus
// the header and footer lines, minus the content box's border and
// padding, falling back to a sane default before the first
// WindowSizeMsg arrives (or if the terminal never reports one, e.g.
// when output is not a TTY).
func (a App) dims() resizeMsg {
	w, h := a.w, a.h-2
	if w <= 0 {
		w = 80
	}
	if h < 1 {
		h = 22
	}
	return resizeMsg{max(w-4, 1), max(h-2, 1)}
}

// header renders the full-width top bar: app name, address and masked
// token on the left, the current page's breadcrumb on the right.
func (a App) header() string {
	w := a.w
	if w <= 0 {
		w = 80
	}
	left := "vaultui " + dimStyle.Render(a.addr) + "  " + dimStyle.Render(a.token)
	right := breadStyle.Render(a.top().Header())
	pad := w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		return topBarStyle.Render(left + " " + right)
	}
	return topBarStyle.Render(left + strings.Repeat(" ", pad) + right)
}

func (a App) View() string {
	header := a.header()
	content := contentStyle.Height(max(a.h-4, 1)).Render(a.top().View())
	footer := bottomStyle.Render(renderHints(a.top().Hints()))
	return header + "\n" + content + "\n" + footer
}

func maskToken(t string) string {
	if len(t) > 8 {
		return t[:4] + "..." + t[len(t)-4:]
	}
	return "****"
}
