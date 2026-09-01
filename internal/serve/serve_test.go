package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestSSHKeysEndpoint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(sshDir, "keys"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Seed real keys plus common non-key entries that must be excluded,
	// including rotated backups (known_hosts.old) and a "password" file.
	for _, name := range []string{
		"id_ed25519", "id_rsa", "id_rsa.pub",
		"known_hosts", "known_hosts.old", "authorized_keys", "authorized_keys.old",
		"config", "password",
	} {
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte("test"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(sshDir, "keys", "gcp.pem"), []byte("test"), 0o600); err != nil {
		t.Fatalf("write gcp.pem: %v", err)
	}
	// A file outside ~/.ssh must never show up in the listing.
	if err := os.WriteFile(filepath.Join(home, "outside.pem"), []byte("test"), 0o600); err != nil {
		t.Fatalf("write outside.pem: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sshkeys", nil)
	recorder := httptest.NewRecorder()
	handleSSHKeys(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		SSHDir string   `json:"ssh_dir"`
		Keys   []string `json:"keys"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SSHDir != sshDir {
		t.Fatalf("unexpected ssh_dir %q, want %q", resp.SSHDir, sshDir)
	}
	want := []string{
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_rsa"),
		filepath.Join(sshDir, "keys", "gcp.pem"),
	}
	if !reflect.DeepEqual(resp.Keys, want) {
		t.Fatalf("unexpected keys %v, want %v", resp.Keys, want)
	}
}

func TestSSHKeySelectEndpointGuards(t *testing.T) {
	// GET is not allowed — only POST triggers the native dialog.
	req := httptest.NewRequest(http.MethodGet, "/api/sshkeys/select", nil)
	recorder := httptest.NewRecorder()
	handleSSHKeySelect(recorder, req)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", recorder.Code)
	}

	// A non-loopback client must be rejected without showing a dialog.
	req = httptest.NewRequest(http.MethodPost, "/api/sshkeys/select", nil)
	req.RemoteAddr = "192.168.1.10:54321"
	recorder = httptest.NewRecorder()
	handleSSHKeySelect(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback client, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "服务器本机") {
		t.Fatalf("unexpected error body: %s", recorder.Body.String())
	}
}

func TestPickerScripts(t *testing.T) {
	script := windowsSSHKeyPickerScript(`C:\Users\me\.ssh`)
	for _, want := range []string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"OpenFileDialog",
		"InitialDirectory = 'C:\\Users\\me\\.ssh'",
		"ShowDialog",
		"$d.FileName",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("windows script missing %q:\n%s", want, script)
		}
	}
	// Single quotes in the start dir must be escaped for PowerShell.
	if !strings.Contains(windowsSSHKeyPickerScript(`C:\it's`), "''") {
		t.Fatal("windows script must escape single quotes in the start dir")
	}

	mac := macSSHKeyPickerScript(`/Users/me/.ssh`)
	for _, want := range []string{"choose file", "POSIX path of f", "default location"} {
		if !strings.Contains(mac, want) {
			t.Fatalf("mac script missing %q:\n%s", want, mac)
		}
	}
	if !strings.Contains(macSSHKeyPickerScript(`/a"b`), `\"`) {
		t.Fatal("mac script must escape double quotes in the start dir")
	}
}

func TestPickerOutputClassification(t *testing.T) {
	// Success: chosen path, trimmed.
	path, err := pickerOutput([]byte("C:\\Users\\me\\.ssh\\id_ed25519\r\n"), nil)
	if err != nil || path != `C:\Users\me\.ssh\id_ed25519` {
		t.Fatalf("unexpected success result: %q, %v", path, err)
	}
	// Cancel with no output and zero exit (Windows PowerShell).
	path, err = pickerOutput(nil, nil)
	if err != nil || path != "" {
		t.Fatalf("expected empty cancel result, got %q, %v", path, err)
	}
	// zenity cancel: exit 1 with no output.
	zenityErr := &exec.ExitError{ProcessState: &os.ProcessState{}, Stderr: nil}
	path, err = pickerOutput(nil, zenityErr)
	if err != nil || path != "" {
		t.Fatalf("expected zenity cancel to be treated as cancel, got %q, %v", path, err)
	}
	// osascript cancel: exit 1 with "User canceled." on stderr.
	osaErr := &exec.ExitError{ProcessState: &os.ProcessState{}, Stderr: []byte("User canceled.")}
	path, err = pickerOutput(nil, osaErr)
	if err != nil || path != "" {
		t.Fatalf("expected osascript cancel to be treated as cancel, got %q, %v", path, err)
	}
	// Helper missing: a real error, not a cancel.
	if _, err := pickerOutput(nil, fmt.Errorf("executable file not found")); err == nil {
		t.Fatal("expected an error for a missing helper")
	}
}
