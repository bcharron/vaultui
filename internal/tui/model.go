package tui

import (
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
// page, mirroring tea.Model semantics.
type Page interface {
	Init() tea.Cmd
	Update(tea.Msg) (Page, tea.Cmd)
	View() string
	Header() string
	Hints() string
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
		case "ctrl+c", "q":
			return a, tea.Quit
		case "esc":
			if len(a.stack) > 1 {
				a.stack = a.stack[:len(a.stack)-1]
				return a, nil
			}
			return a, tea.Quit
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

// dims returns the current content-area dimensions, falling back to a
// sane default before the first WindowSizeMsg arrives (or if the
// terminal never reports one, e.g. when output is not a TTY).
func (a App) dims() resizeMsg {
	w, h := a.w, a.h-2
	if w <= 0 {
		w = 80
	}
	if h < 1 {
		h = 22
	}
	return resizeMsg{w, h}
}

func (a App) View() string {
	header := topBarStyle.Render("vaultui") + " " +
		dimStyle.Render(a.addr+"  "+a.token) +
		breadStyle.Render("  "+a.top().Header())
	content := lipgloss.NewStyle().Width(a.w).Height(max(a.h-2, 1)).Render(a.top().View())
	footer := bottomStyle.Render(a.top().Hints())
	return header + "\n" + content + "\n" + footer
}

func maskToken(t string) string {
	if len(t) > 8 {
		return t[:4] + "..." + t[len(t)-4:]
	}
	return "****"
}
