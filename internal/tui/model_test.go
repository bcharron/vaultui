package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// barBackground reports whether the escape sequence immediately
// preceding s in h sets the header bar background.
func barBackground(h, s string) bool {
	i := strings.Index(h, s)
	if i < 0 {
		return false
	}
	esc := strings.LastIndexByte(h[:i], '\x1b')
	if esc < 0 {
		return false
	}
	return strings.Contains(h[esc:i], "48;5;62")
}

// barForeground reports whether the escape sequence immediately
// preceding s in h sets a light foreground fit for the bar background.
func barForeground(h, s string) bool {
	i := strings.Index(h, s)
	if i < 0 {
		return false
	}
	esc := strings.LastIndexByte(h[:i], '\x1b')
	if esc < 0 {
		return false
	}
	return strings.Contains(h[esc:i], "38;5;252") || strings.Contains(h[esc:i], "38;5;230")
}

func TestTokenRefreshDue(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name              string
		anchor, lastProbe time.Time
		want              bool
	}{
		{"fresh anchor", now.Add(-10 * time.Second), time.Time{}, false},
		{"anchor a minute old, never probed", now.Add(-61 * time.Second), time.Time{}, true},
		{"probed recently", now.Add(-70 * time.Second), now.Add(-5 * time.Second), false},
		{"probe also a minute old", now.Add(-70 * time.Second), now.Add(-61 * time.Second), true},
		{"no anchor yet", time.Time{}, time.Time{}, false},
	}
	for _, c := range cases {
		if got := tokenRefreshDue(c.anchor, c.lastProbe, now); got != c.want {
			t.Errorf("%s: tokenRefreshDue = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestHeaderBarBackground pins the header bar's rendering: with a
// forced color profile, every visible segment must set the bar
// background (a wrapping style would lose it at the first embedded
// segment's ANSI reset) and a light foreground (dim greys are
// unreadable on the mid-purple bar).
func TestHeaderBarBackground(t *testing.T) {
	r := lipgloss.DefaultRenderer()
	old := r.ColorProfile()
	r.SetColorProfile(termenv.TrueColor)
	defer r.SetColorProfile(old)

	a := App{
		addr:        "http://127.0.0.1:8200",
		clusterName: "vault-cluster-x",
		tokenTTL:    0,
		tokenAt:     time.Now(),
		w:           120,
		stack:       []Page{newHomePage(nil)},
	}
	h := a.header()

	for _, seg := range []string{"http://127.0.0.1:8200", "cluster: vault-cluster-x", "token: no expiry", "home"} {
		if !barBackground(h, seg) {
			t.Errorf("segment %q has no bar background\n%q", seg, h)
		}
		if !barForeground(h, seg) {
			t.Errorf("segment %q has no light foreground\n%q", seg, h)
		}
	}
	// the fill between segments must be painted too
	if n := strings.Count(h, "48;5;62"); n < 6 {
		t.Errorf("only %d background sequences in header, want >= 6\n%q", n, h)
	}
	// dark greys must never land on the bar
	if strings.Contains(h, "38;5;241;48;5;62") {
		t.Errorf("dark grey on the bar background\n%q", h)
	}
}
