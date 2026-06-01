package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mragab/jtx/internal/config"
	"github.com/mragab/jtx/internal/jira"
	"github.com/mragab/jtx/internal/ui"
)

func main() {
	jql := flag.String("jql", "", "JQL query for issues to display (default: project board or assignee)")
	cfgDir := flag.String("config-dir", configDir(), "Directory containing config.toml or config.yaml")
	flag.Parse()

	cfg, err := config.Load(*cfgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
	_ = config.WriteDefault(*cfgDir)

	// Resolution order: -jql flag > config jql > project default > assignee fallback
	query := *jql
	if query == "" {
		query = cfg.JQL
	}
	if query == "" {
		if cfg.Project != "" {
			query = fmt.Sprintf("project = %s ORDER BY updated DESC", cfg.Project)
		} else {
			query = "assignee = currentUser() ORDER BY updated DESC"
		}
	}

	client, err := jira.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\nRequired environment variables:\n  JIRA_API_TOKEN - Atlassian API token\n  JIRA_EMAIL     - Atlassian account email\n  JIRA_URL       - Jira instance base URL (e.g. https://your-org.atlassian.net)\n\nOptional:\n  PROJECT_ID     - overrides project in config\n", err)
		os.Exit(1)
	}

	m := ui.New(client, query, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "jtx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "jtx")
}
