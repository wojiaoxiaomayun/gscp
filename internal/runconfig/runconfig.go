package runconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = ".genv"

const defaultTemplate = `{
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

func LoadFromDir(dir string) (map[string]Target, string, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}

	targets := map[string]Target{}
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, path, fmt.Errorf("parse %s: %w", FileName, err)
	}

	return targets, path, nil
}

func InitInDir(dir string, alias string) (string, error) {
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", FileName)
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
