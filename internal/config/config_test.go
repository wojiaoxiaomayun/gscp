package config

import (
	"os"
	"strings"
	"testing"
)

func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestParseSupportsPasswordAndPrivateKeyAuthentication(t *testing.T) {
	store, err := Parse([]byte(`{
  "servers": {
    "password-only": {
      "host": "10.0.0.1",
      "username": "root",
      "password": "secret"
    },
    "key-only": {
      "alias": "key-only",
      "host": "10.0.0.2",
      "username": "deploy",
      "key_path": " ~/.ssh/id_ed25519 ",
      "key_pass": " key secret "
    },
    "fallback": {
      "alias": "fallback",
      "host": "10.0.0.3",
      "username": "deploy",
      "password": " password with spaces ",
      "key_path": "~/.ssh/id_rsa"
    }
  }
}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if got := store.Servers["password-only"].Alias; got != "password-only" {
		t.Fatalf("expected fallback alias, got %q", got)
	}
	if got := store.Servers["key-only"].KeyPath; got != "~/.ssh/id_ed25519" {
		t.Fatalf("expected normalized key path, got %q", got)
	}
	if got := store.Servers["key-only"].KeyPass; got != " key secret " {
		t.Fatalf("key passphrase was modified: %q", got)
	}
	if got := store.Servers["fallback"].Password; got != " password with spaces " {
		t.Fatalf("password was modified: %q", got)
	}
}

func TestParseRejectsInvalidAuthentication(t *testing.T) {
	tests := []struct {
		name    string
		server  string
		wantErr string
	}{
		{
			name:    "missing authentication",
			server:  `{"alias":"bad","host":"host","username":"user"}`,
			wantErr: "either password or key_path",
		},
		{
			name:    "blank authentication",
			server:  `{"alias":"bad","host":"host","username":"user","password":"   ","key_path":"  "}`,
			wantErr: "either password or key_path",
		},
		{
			name:    "orphan key passphrase",
			server:  `{"alias":"bad","host":"host","username":"user","password":"secret","key_pass":"passphrase"}`,
			wantErr: "key_pass requires key_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(`{"servers":{"bad":` + tt.server + `}}`))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSaveValidatesBeforeWriting(t *testing.T) {
	useTempConfigDir(t)
	store := &Store{Servers: map[string]Server{
		"bad": {Alias: "bad", Host: "host", Username: "user"},
	}}

	err := store.Save()
	if err == nil || !strings.Contains(err.Error(), "either password or key_path") {
		t.Fatalf("expected authentication validation error, got %v", err)
	}

	path, pathErr := Path()
	if pathErr != nil {
		t.Fatalf("config path: %v", pathErr)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid config should not be written, stat error: %v", statErr)
	}
}
