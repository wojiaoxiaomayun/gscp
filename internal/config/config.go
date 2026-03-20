package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

type Store struct {
	Servers map[string]Server `json:"servers"`
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

	store := &Store{}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if store.Servers == nil {
		store.Servers = map[string]Server{}
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
	s.Servers[server.Alias] = server
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
