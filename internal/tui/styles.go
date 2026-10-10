package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	topBarStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	breadStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	bottomStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	itemStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	dirStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	typeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	loadingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	activeTabStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
)

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
