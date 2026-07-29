package cli

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"gscp/internal/config"
	"gscp/internal/runconfig"
)

func TestSortedEnvKeys(t *testing.T) {
	targets := map[string]runconfig.Target{
		"pro":  {},
		"dev":  {},
		"test": {},
	}

	got := sortedEnvKeys(targets)
	want := []string{"dev", "pro", "test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected env keys: got %v want %v", got, want)
	}
}

func TestSelectDefaultEnv(t *testing.T) {
	targets := map[string]runconfig.Target{
		"dev": {IsDefault: true},
		"pro": {},
	}

	got, err := selectDefaultEnv(targets)
	if err != nil {
		t.Fatalf("select default env: %v", err)
	}
	if got != "dev" {
		t.Fatalf("unexpected default env: %s", got)
	}
}

func TestSelectDefaultEnvRejectsMultipleDefaults(t *testing.T) {
	targets := map[string]runconfig.Target{
		"dev": {IsDefault: true},
		"pro": {IsDefault: true},
	}

	if _, err := selectDefaultEnv(targets); err == nil {
		t.Fatal("expected multiple defaults error")
	}
}

func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestRunAddAuthenticationModes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want config.Server
	}{
		{
			name: "password",
			args: []string{"password", "host", "root", " password secret "},
			want: config.Server{Alias: "password", Host: "host", Username: "root", Password: " password secret "},
		},
		{
			name: "private key",
			args: []string{"-i", " ~/.ssh/id_ed25519 ", "-P", " key secret ", "key", "host", "deploy"},
			want: config.Server{Alias: "key", Host: "host", Username: "deploy", KeyPath: "~/.ssh/id_ed25519", KeyPass: " key secret "},
		},
		{
			name: "private key with password fallback",
			args: []string{"-i", "~/.ssh/id_rsa", "fallback", "host", "deploy", " fallback secret "},
			want: config.Server{Alias: "fallback", Host: "host", Username: "deploy", Password: " fallback secret ", KeyPath: "~/.ssh/id_rsa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempConfigDir(t)
			if err := runAdd(tt.args); err != nil {
				t.Fatalf("run add: %v", err)
			}
			store, err := config.Load()
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if got := store.Servers[tt.want.Alias]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected server: got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestRunAddRejectsPassphraseWithoutKey(t *testing.T) {
	useTempConfigDir(t)
	err := runAdd([]string{"-P", "passphrase", "alias", "host", "root", "password"})
	if err == nil || !strings.Contains(err.Error(), "-P requires -i") {
		t.Fatalf("expected -P validation error, got %v", err)
	}
	store, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if len(store.Servers) != 0 {
		t.Fatalf("invalid command should not save servers: %+v", store.Servers)
	}
}

func TestRunAddRemoteMergesServers(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	store, err := config.Load()
	if err != nil {
		t.Fatalf("load local store: %v", err)
	}
	store.Upsert(config.Server{Alias: "local", Host: "127.0.0.1", Username: "root", Password: "local-pass"})
	if err := store.Save(); err != nil {
		t.Fatalf("save local store: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "servers": {
    "local": {
      "alias": "local",
      "host": "10.0.0.2",
      "username": "admin",
      "password": "remote-pass"
    },
    "remote": {
      "alias": "remote",
      "host": "10.0.0.3",
      "username": "deploy",
      "password": "secret"
    }
  }
}`))
	}))
	defer server.Close()

	if err := runAdd([]string{"-r", server.URL}); err != nil {
		t.Fatalf("run add remote: %v", err)
	}

	merged, err := config.Load()
	if err != nil {
		t.Fatalf("load merged store: %v", err)
	}
	if len(merged.Servers) != 2 {
		t.Fatalf("unexpected server count: %d", len(merged.Servers))
	}
	if merged.Servers["local"].Host != "10.0.0.2" {
		t.Fatalf("expected remote server to overwrite local host, got %s", merged.Servers["local"].Host)
	}
	if merged.Servers["remote"].Username != "deploy" {
		t.Fatalf("expected remote server to be merged, got %+v", merged.Servers["remote"])
	}
}
