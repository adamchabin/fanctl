package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"fanctl/internal/config"
	"fanctl/internal/hwmon"
	"fanctl/internal/ui"
)

func main() {
	apiPort := 8080
	if cfg, err := config.LoadConfig("/etc/fanctl/fanctl.conf"); err == nil && cfg.APIPort != 0 {
		apiPort = cfg.APIPort
	}

	scanner := hwmon.NewScanner()

	model := ui.NewModel(scanner, apiPort)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI client: %v\n", err)
		os.Exit(1)
	}
}
