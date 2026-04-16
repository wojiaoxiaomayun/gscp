package runconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = ".genv"

// ErrAlreadyExists is returned by InitInDir when .genv already exists.
var ErrAlreadyExists = errors.New(".genv already exists")

const defaultTemplate = `{
  "groups": {
    "default": ["dev"]
  },
  "dev": {
    "active_alias": "%s",
    "is_default": true,
    "local_path": "./dist",
    "to_path": "/var/www/app",
    "commands": [
      "cd /var/www/app",
      "sudo systemctl restart app"
    ]
  }
}
`

type Target struct {
	ActiveAlias string   `json:"active_alias"`
	IsDefault   bool     `json:"is_default"`
	LocalPath   string   `json:"local_path"`
	ToPath      string   `json:"to_path"`
	Commands    []string `json:"commands"`
}

type ConfigFile struct {
	Targets map[string]Target
	Groups  map[string][]string
}

func LoadConfigFromDir(dir string) (*ConfigFile, string, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, path, fmt.Errorf("parse %s: %w", FileName, err)
	}

	cfg := &ConfigFile{
		Targets: map[string]Target{},
		Groups:  map[string][]string{},
	}

	for key, value := range raw {
		if key == "groups" {
			if err := json.Unmarshal(value, &cfg.Groups); err != nil {
				return nil, path, fmt.Errorf("parse %s groups: %w", FileName, err)
			}
			continue
		}

		var target Target
		if err := json.Unmarshal(value, &target); err != nil {
			return nil, path, fmt.Errorf("parse %s target %q: %w", FileName, key, err)
		}
		cfg.Targets[key] = target
	}

	return cfg, path, nil
}

func LoadFromDir(dir string) (map[string]Target, string, error) {
	cfg, path, err := LoadConfigFromDir(dir)
	if err != nil {
		return nil, path, err
	}
	return cfg.Targets, path, nil
}

func InitInDir(dir string, alias string) (string, error) {
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); err == nil {
		return "", ErrAlreadyExists
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check %s: %w", FileName, err)
	}

	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "your-server-alias"
	}

	template := fmt.Sprintf(defaultTemplate, alias)
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", FileName, err)
	}

	return path, nil
}
