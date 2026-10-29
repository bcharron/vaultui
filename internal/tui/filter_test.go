package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeFilter(t *testing.T, f *filter, keys ...string) {
	t.Helper()
	for _, k := range keys {
		f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
}

func TestFilterMatch(t *testing.T) {
	var f filter
	if !f.match("anything") {
		t.Error("empty query must match everything")
	}
	f.active = true
	typeFilter(t, &f, "a", "p", "p", "s")
	for _, s := range []string{"apps", "Apps/", "api-apps"} {
		if !f.match(s) {
			t.Errorf("filter %q should match %q", f.q, s)
		}
	}
	for _, s := range []string{"db", "web", "misc"} {
		if f.match(s) {
			t.Errorf("filter %q should not match %q", f.q, s)
		}
	}
}

func TestFilterKeys(t *testing.T) {
	var f filter
	f.active = true
	if !f.typing() {
		t.Fatal("filter should be typing while active")
	}
	// q and esc are captured while open, not quit/back
	typeFilter(t, &f, "k", "e", "y")
	if f.q != "key" {
		t.Errorf("query = %q, want %q", f.q, "key")
	}
	if !f.update(tea.KeyMsg{Type: tea.KeyBackspace}) {
		t.Error("backspace should be handled by the filter")
	}
	if f.q != "ke" {
		t.Errorf("query after backspace = %q, want %q", f.q, "ke")
	}
	// enter keeps the query but closes input capture
	if !f.update(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Error("enter should be handled by the filter")
	}
	if f.typing() {
		t.Error("enter should close the filter")
	}
	if f.q != "ke" {
		t.Errorf("enter must keep the query, got %q", f.q)
	}
	// esc clears it
	f.active = true
	if !f.update(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Error("esc should be handled by the filter")
	}
	if f.q != "" || f.typing() {
		t.Errorf("esc should clear the filter, got q=%q active=%v", f.q, f.active)
	}
	// navigation keys fall through while open
	f.active = true
	if f.update(keyMsg("down")) {
		t.Error("navigation keys must fall through to the page")
	}
}
