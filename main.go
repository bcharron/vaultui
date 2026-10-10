package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"vaultui/internal/tui"
	"vaultui/internal/vault"
)

func main() {
	cl, err := vault.NewClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "vaultui:", err)
		os.Exit(1)
	}
	p := tea.NewProgram(tui.New(cl), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "vaultui:", err)
		os.Exit(1)
	}
}
