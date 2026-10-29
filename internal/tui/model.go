package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/bcharron/vaultui/internal/vault"
)

// resizeMsg carries the content-area dimensions (width x height) to every
// page whenever the terminal is resized.
type resizeMsg struct{ w, h int }

// pushMsg/popMsg drive the navigation stack.
type pushMsg struct{ page Page }
type popMsg struct{}

// tickMsg drives the token countdown: the header recomputes the
// estimated time left from the launch TTL on every tick.
type tickMsg time.Time

// tickEvery returns a command that delivers a tickMsg after d. Do not
// feed tickMsg into the test harness's drive loop: each tick schedules
// the next, which would spin it forever.
func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// appHealthMsg and appTokenMsg carry the header's background fetches:
// sys/health for the cluster name, auth/token/lookup-self for the
// token expiry countdown.
type appHealthMsg struct {
	health map[string]interface{}
	err    error
}

type appTokenMsg struct {
	data map[string]interface{}
	err  error
}

// tokenRefreshDue reports whether the periodic lookup-self re-fetch is
// due: a minute after the last successful anchor, at most one attempt
// per minute even when attempts fail.
func tokenRefreshDue(anchor, lastProbe, now time.Time) bool {
	return !anchor.IsZero() &&
		now.Sub(anchor) >= time.Minute &&
		(lastProbe.IsZero() || now.Sub(lastProbe) >= time.Minute)
}

// tokenTTLFrom extracts the remaining TTL in seconds from lookup-self
// data; -1 when unknown.
func tokenTTLFrom(data map[string]interface{}) int {
	if ttl, ok := data["ttl"].(float64); ok {
		return int(ttl)
	}
	return -1
}

// tokenExpiryLabel renders the token's remaining validity, counting
// down from a TTL fetched at at: "28m30s left", "no expiry", or ""
// when unknown.
func tokenExpiryLabel(ttl int, at time.Time) string {
	switch {
	case ttl < 0:
		return ""
	case ttl == 0:
		return "no expiry"
	}
	rem := ttl - int(time.Since(at).Seconds())
	if rem <= 0 {
		return "expired"
	}
	return humanDuration(rem) + " left"
}

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
	cl           *vaultapi.Client
	addr         string
	ns           string
	clusterName  string
	tokenTTL     int
	tokenAt      time.Time
	tokenProbeAt time.Time // last periodic lookup-self attempt
	stack        []Page
	sp           spinner.Model
	w, h         int
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
	cl := a.cl
	return tea.Batch(
		a.top().Init(),
		a.sp.Tick,
		tickEvery(time.Second),
		func() tea.Msg {
			health, err := vault.Health(cl)
			return appHealthMsg{health: health, err: err}
		},
		func() tea.Msg {
			data, err := vault.LookupSelf(cl)
			return appTokenMsg{data: data, err: err}
		},
	)
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
		case "ctrl+l":
			// full repaint, for when something else scribbled on the
			// terminal or the display drifted
			return a, tea.ClearScreen
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

	case appHealthMsg:
		if msg.err == nil {
			if n, ok := msg.health["cluster_name"].(string); ok {
				a.clusterName = n
			}
		}
		return a, nil

	case appTokenMsg:
		// The lookup anchors the countdown of the token expiry shown
		// in the header.
		if msg.err == nil {
			a.tokenTTL = tokenTTLFrom(msg.data)
			a.tokenAt = time.Now()
		}
		return a, nil

	case tickMsg:
		cmds := []tea.Cmd{tickEvery(time.Second)}
		// Re-anchor the countdown every minute with a fresh lookup, so
		// renewed or otherwise changed TTLs are picked up. Attempts are
		// capped at one per minute even when they fail.
		if tokenRefreshDue(a.tokenAt, a.tokenProbeAt, time.Now()) {
			a.tokenProbeAt = time.Now()
			cl := a.cl
			cmds = append(cmds, func() tea.Msg {
				data, err := vault.LookupSelf(cl)
				return appTokenMsg{data: data, err: err}
			})
		}
		return a, tea.Batch(cmds...)

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
	if !a.tokenAt.IsZero() {
		if l := tokenExpiryLabel(a.tokenTTL, a.tokenAt); l != "" {
			left += topBarInfo.Render("  token: " + l)
		}
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
