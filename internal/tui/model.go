package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
// (header with address, cluster and token expiry, footer with spinner
// and key hints).
type App struct {
	cl          *vaultapi.Client
	addr        string
	ns          string
	clusterName string
	tokenExpiry string
	stack       []Page
	sp          spinner.Model
	w, h        int
}

// New builds the app around an authenticated Vault client.
func New(cl *vaultapi.Client) App {
	return App{
		cl:    cl,
		addr:  cl.Address(),
		ns:    cl.Namespace(),
		stack: []Page{newHomePage(cl)},
		sp:    spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("117")))),
	}
}

func (a App) top() Page { return a.stack[len(a.stack)-1] }

func (a App) Init() tea.Cmd {
	return tea.Batch(a.top().Init(), a.sp.Tick)
}

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

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.sp, cmd = a.sp.Update(msg)
		return a, cmd

	case homeHealthMsg:
		// The home screen's fetch also feeds the header's cluster name.
		if msg.err == nil {
			if n, ok := msg.health["cluster_name"].(string); ok {
				a.clusterName = n
			}
		}
		return a.broadcast(msg)

	case homeTokenMsg:
		// ... and its lookup-self feeds the header's token expiry.
		if msg.err == nil {
			a.tokenExpiry = tokenExpiryLabel(msg.data)
		}
		return a.broadcast(msg)

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

// header renders the full-width top bar: app name and address on the
// left with the cluster name and token expiry once they are known,
// the current page's breadcrumb on the right. Every segment carries
// the bar background explicitly: a single wrapping style would lose
// it at the first embedded segment's ANSI reset.
func (a App) header() string {
	w := a.w
	if w <= 0 {
		w = 80
	}
	left := topBarStyle.Render("vaultui") + topBarInfo.Render(" "+a.addr)
	if a.clusterName != "" {
		left += topBarInfo.Render("  cluster: " + a.clusterName)
	}
	if a.tokenExpiry != "" {
		left += topBarInfo.Render("  token: " + a.tokenExpiry)
	}
	right := topBarCrumb.Render(a.top().Header())
	pad := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if pad < 1 {
		return left + topBarBg.Render(" ") + right
	}
	return left + topBarBg.Render(strings.Repeat(" ", pad)) + right + topBarBg.Render(" ")
}

// loading reports whether the top page is waiting on data, via the
// optional Loading implementation.
func (a App) loading() bool {
	l, ok := a.top().(interface{ Loading() bool })
	return ok && l.Loading()
}

func (a App) View() string {
	header := a.header()
	content := contentStyle.Height(max(a.h-4, 1)).Render(a.top().View())
	footer := bottomStyle.Render(renderHints(a.top().Hints()))
	if a.loading() {
		footer = a.sp.View() + " " + footer
	}
	return header + "\n" + content + "\n" + footer
}
