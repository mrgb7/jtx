package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Column struct {
	Title    string   `toml:"title" yaml:"title"`
	Color    string   `toml:"color" yaml:"color"`
	Statuses []string `toml:"statuses" yaml:"statuses"`
}

type Config struct {
	Project string   `toml:"project" yaml:"project"`
	JQL     string   `toml:"jql" yaml:"jql"`
	Columns []Column `toml:"columns" yaml:"columns"`
}

// Default mirrors the current hard-coded board.
func Default() *Config {
	return &Config{
		Project: "",
		Columns: []Column{
			{
				Title: "Selected for Dev",
				Color: "todo",
				Statuses: []string{
					"selected for development",
					"selected for dev",
				},
			},
			{
				Title: "In Progress",
				Color: "inprogress",
				Statuses: []string{
					"in progress",
				},
			},
			{
				Title: "Reviewing",
				Color: "review",
				Statuses: []string{
					"reviewing",
					"in review",
					"ready to deploy",
					"code review",
				},
			},
			{
				Title: "Done",
				Color: "done",
				Statuses: []string{
					"done",
					"closed",
					"resolved",
					"released",
					"completed",
					"unresolved",
				},
			},
			{
				Title: "Blocked",
				Color: "blocked",
				Statuses: []string{
					"blocked",
					"on hold",
					"impediment",
				},
			},
		},
	}
}

// Load reads config.toml or config.yaml from the given directory,
// falling back to Default if neither exists.
// PROJECT_ID env var always overrides the project field.
func Load(dir string) (*Config, error) {
	var cfg *Config
	for _, name := range []string{"config.toml", "config.yaml", "config.yml"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		c, err := parse(path, data)
		if err != nil {
			return nil, fmt.Errorf("invalid config %s: %w", path, err)
		}
		if len(c.Columns) == 0 {
			return nil, fmt.Errorf("config %s has no columns defined", path)
		}
		cfg = c
		break
	}
	if cfg == nil {
		cfg = Default()
	}
	// ENV always wins
	if p := os.Getenv("PROJECT_ID"); p != "" {
		cfg.Project = p
	}
	return cfg, nil
}

func parse(path string, data []byte) (*Config, error) {
	var cfg Config
	switch filepath.Ext(path) {
	case ".toml":
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format")
	}
	return &cfg, nil
}

// WriteDefault writes the default config as TOML to dir/config.toml.
func WriteDefault(dir string) error {
	path := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(Default())
}
