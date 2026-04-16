package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	configDirName  = "gscp"
	configFileName = "servers.json"
)

var ErrServerNotFound = errors.New("server not found")

type Server struct {
	Alias    string `json:"alias"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

<<<<<<< HEAD
// ScanSettings holds configuration for the local .genv file scanner.
type ScanSettings struct {
	// SkipDirs is a list of directory names to skip during scanning.
	SkipDirs []string `json:"skip_dirs,omitempty"`
	// ScanRoots is a list of root paths to scan. If empty, the user home dir is used.
	ScanRoots []string `json:"scan_roots,omitempty"`
}

// DefaultSkipDirs returns the built-in list of directories to skip.
func DefaultSkipDirs() []string {
	return []string{
		".git", ".svn", ".hg",
		".idea", ".vscode", ".vs",
		"node_modules", "vendor",
		".cache", ".npm", ".yarn", ".pnpm-store",
		"__pycache__", ".tox", ".venv", "venv", "env",
		"dist", "build", "out", "target",
		".gradle", ".m2",
		"Library", "Applications",
	}
}

type Store struct {
	Servers      map[string]Server `json:"servers"`
	Workspaces   []string          `json:"workspaces,omitempty"`
	ScanSettings *ScanSettings     `json:"scan_settings,omitempty"`
=======
type Store struct {
	Servers    map[string]Server `json:"servers"`
	Workspaces []string          `json:"workspaces,omitempty"`
>>>>>>> a6686846fbbc609b642a6d767a29340b61ae1934
}

func Load() (*Store, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{Servers: map[string]Server{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	store, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func Parse(data []byte) (*Store, error) {
	store := &Store{}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if store.Servers == nil {
		store.Servers = map[string]Server{}
	}

	for alias, server := range store.Servers {
		if err := validateServer(server, alias); err != nil {
			return nil, err
		}
		if strings.TrimSpace(server.Alias) == "" {
			server.Alias = alias
		}
		store.Servers[alias] = normalizeServer(server)
	}

	return store, nil
}

func (s *Store) Save() error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (s *Store) Upsert(server Server) {
	if s.Servers == nil {
		s.Servers = map[string]Server{}
	}
	server = normalizeServer(server)
	s.Servers[server.Alias] = server
}

// AddWorkspace appends dir to the workspace list if it is not already present.
func (s *Store) AddWorkspace(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	for _, w := range s.Workspaces {
		if w == dir {
			return
		}
	}
	s.Workspaces = append(s.Workspaces, dir)
}

func (s *Store) Merge(other *Store) {
	if other == nil {
		return
	}
	if s.Servers == nil {
		s.Servers = map[string]Server{}
	}
	for _, server := range other.Servers {
		s.Upsert(server)
	}
}

func (s *Store) Remove(alias string) error {
	if _, ok := s.Servers[alias]; !ok {
		return ErrServerNotFound
	}

	delete(s.Servers, alias)
	return nil
}

func (s *Store) List() []Server {
	servers := make([]Server, 0, len(s.Servers))
	for _, server := range s.Servers {
		servers = append(servers, server)
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Alias < servers[j].Alias
	})

	return servers
}

<<<<<<< HEAD
// GetScanSettings returns the effective scan settings, merging defaults with stored config.
func (s *Store) GetScanSettings() ScanSettings {
	defaults := DefaultSkipDirs()
	if s.ScanSettings == nil {
		return ScanSettings{SkipDirs: defaults}
	}
	ss := *s.ScanSettings
	if len(ss.SkipDirs) == 0 {
		ss.SkipDirs = defaults
	}
	return ss
}

=======
>>>>>>> a6686846fbbc609b642a6d767a29340b61ae1934
func Path() (string, error) {
	return configFilePath()
}

func configFilePath() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}

	return filepath.Join(configRoot, configDirName, configFileName), nil
}

func validateServer(server Server, fallbackAlias string) error {
	alias := strings.TrimSpace(server.Alias)
	if alias == "" {
		alias = strings.TrimSpace(fallbackAlias)
	}
	if alias == "" || strings.TrimSpace(server.Host) == "" || strings.TrimSpace(server.Username) == "" || server.Password == "" {
		return errors.New("alias, host, username and password cannot be empty")
	}
	return nil
}

func normalizeServer(server Server) Server {
	server.Alias = strings.TrimSpace(server.Alias)
	server.Host = strings.TrimSpace(server.Host)
	server.Username = strings.TrimSpace(server.Username)
	return server
}
