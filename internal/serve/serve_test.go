package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gscp/internal/config"
)

func useTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func saveServer(t *testing.T, server config.Server) {
	t.Helper()
	store, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	store.Upsert(server)
	if err := store.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func requestServers(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	if strings.HasPrefix(target, "/api/servers/") {
		handleServerByAlias(recorder, req)
	} else {
		handleServers(recorder, req)
	}
	return recorder
}

func TestServerResponsesDoNotExposeSecrets(t *testing.T) {
	useTempConfigDir(t)
	saveServer(t, config.Server{
		Alias:    "prod",
		Host:     "host",
		Username: "root",
		Password: "password-secret",
		KeyPath:  "~/.ssh/id_ed25519",
		KeyPass:  "key-secret",
	})

	response := requestServers(t, http.MethodGet, "/api/servers", "")
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"password-secret", "key-secret", `"password"`, `"key_pass"`} {
		if strings.Contains(body, secret) {
			t.Fatalf("response exposed %q: %s", secret, body)
		}
	}

	var servers []serverResponse
	if err := json.Unmarshal(response.Body.Bytes(), &servers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(servers) != 1 || !servers[0].HasPassword || !servers[0].HasKeyPass {
		t.Fatalf("unexpected public response: %+v", servers)
	}
}

func TestUpdateServerPreservesOmittedSecrets(t *testing.T) {
	useTempConfigDir(t)
	saveServer(t, config.Server{
		Alias:    "prod",
		Host:     "old-host",
		Username: "root",
		Password: "password-secret",
		KeyPath:  "~/.ssh/id_ed25519",
		KeyPass:  "key-secret",
	})

	response := requestServers(t, http.MethodPut, "/api/servers/prod", `{
  "host": "new-host",
  "username": "deploy",
  "key_path": "~/.ssh/id_ed25519"
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}

	store, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	server := store.Servers["prod"]
	if server.Password != "password-secret" || server.KeyPass != "key-secret" {
		t.Fatalf("omitted secrets were not preserved: %+v", server)
	}
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("update response exposed secret: %s", response.Body.String())
	}
}

func TestUpdateServerCanClearCredentials(t *testing.T) {
	useTempConfigDir(t)
	saveServer(t, config.Server{
		Alias:    "prod",
		Host:     "host",
		Username: "root",
		Password: "password-secret",
		KeyPath:  "~/.ssh/id_ed25519",
		KeyPass:  "key-secret",
	})

	response := requestServers(t, http.MethodPut, "/api/servers/prod", `{
  "host": "host",
  "username": "root",
  "password": "",
  "key_pass": ""
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	store, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	server := store.Servers["prod"]
	if server.Password != "" || server.KeyPass != "" || server.KeyPath == "" {
		t.Fatalf("credentials were not cleared as requested: %+v", server)
	}

	response = requestServers(t, http.MethodPut, "/api/servers/prod", `{
  "host": "host",
  "username": "root",
  "key_path": ""
}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "either password or key_path") {
		t.Fatalf("expected clearing last auth method to fail, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateServerValidatesAndRedactsCredentials(t *testing.T) {
	useTempConfigDir(t)
	response := requestServers(t, http.MethodPost, "/api/servers", `{
  "alias": "key-server",
  "host": "host",
  "username": "root",
  "key_path": "~/.ssh/id_ed25519",
  "key_pass": "secret"
}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), `"key_pass"`) {
		t.Fatalf("create response exposed key passphrase: %s", response.Body.String())
	}

	response = requestServers(t, http.MethodPost, "/api/servers", `{
  "alias": "bad",
  "host": "host",
  "username": "root",
  "password": "   ",
  "key_path": "   "
}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected blank credentials to fail, got %d: %s", response.Code, response.Body.String())
	}
}
