package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gscp/internal/config"
	"gscp/internal/runconfig"
)

// Run starts the web server on the given address (e.g. ":8080").
// It blocks until the server exits or ctx is cancelled.
func Run(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/style.css", handleStyleCSS)
	mux.HandleFunc("/app.js", handleAppJS)
	mux.HandleFunc("/api/servers", handleServers)
	mux.HandleFunc("/api/servers/", handleServerByAlias)
	mux.HandleFunc("/api/workspaces", handleWorkspaces)
	mux.HandleFunc("/api/workspaces/add", handleWorkspaceAdd)
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)
	mux.HandleFunc("/api/sshkeys", handleSSHKeys)
	mux.HandleFunc("/api/sshkeys/select", handleSSHKeySelect)
	mux.HandleFunc("/api/scan", handleScan)
	mux.HandleFunc("/api/settings", handleSettings)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	actualAddr := ln.Addr().String()
	fmt.Fprintf(os.Stdout, "gscp serve listening on http://localhost%s\n", portFromAddr(actualAddr))
	fmt.Fprintln(os.Stdout, "Press Ctrl+C to stop.")

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: sshKeyDialogTimeout,
	}

	return srv.Serve(ln)
}

// RunWithContext starts the web server and shuts it down when ctx is cancelled.
func RunWithContext(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/style.css", handleStyleCSS)
	mux.HandleFunc("/app.js", handleAppJS)
	mux.HandleFunc("/api/servers", handleServers)
	mux.HandleFunc("/api/servers/", handleServerByAlias)
	mux.HandleFunc("/api/workspaces", handleWorkspaces)
	mux.HandleFunc("/api/workspaces/add", handleWorkspaceAdd)
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)
	mux.HandleFunc("/api/sshkeys", handleSSHKeys)
	mux.HandleFunc("/api/sshkeys/select", handleSSHKeySelect)
	mux.HandleFunc("/api/scan", handleScan)
	mux.HandleFunc("/api/settings", handleSettings)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	actualAddr := ln.Addr().String()
	fmt.Fprintf(os.Stdout, "gscp serve listening on http://localhost%s\n", portFromAddr(actualAddr))
	fmt.Fprintln(os.Stdout, "Press Ctrl+C to stop.")

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: sshKeyDialogTimeout,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func portFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return ":" + port
}

type serverResponse struct {
	Alias       string `json:"alias"`
	Host        string `json:"host"`
	Username    string `json:"username"`
	KeyPath     string `json:"key_path,omitempty"`
	HasPassword bool   `json:"has_password"`
	HasKeyPass  bool   `json:"has_key_pass"`
}

type serverUpdateRequest struct {
	Host     string  `json:"host"`
	Username string  `json:"username"`
	Password *string `json:"password"`
	KeyPath  *string `json:"key_path"`
	KeyPass  *string `json:"key_pass"`
}

func publicServer(server config.Server) serverResponse {
	return serverResponse{
		Alias:       server.Alias,
		Host:        server.Host,
		Username:    server.Username,
		KeyPath:     server.KeyPath,
		HasPassword: server.Password != "",
		HasKeyPass:  server.KeyPass != "",
	}
}

func publicServers(servers []config.Server) []serverResponse {
	result := make([]serverResponse, 0, len(servers))
	for _, server := range servers {
		result = append(result, publicServer(server))
	}
	return result
}

// handleServers handles GET /api/servers and POST /api/servers
func handleServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, publicServers(store.List()))

	case http.MethodPost:
		var server config.Server
		if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		server = config.NormalizeServer(server)
		if err := config.ValidateServer(server); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		store.Upsert(server)
		if err := store.Save(); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(publicServer(server))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getGitBranch returns the current git branch name for the given directory.
// Returns empty string if git is not available or the directory is not a git repository.
func getGitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(output))
	return branch
}

// handleServerByAlias handles PUT /api/servers/{alias} and DELETE /api/servers/{alias}
func handleServerByAlias(w http.ResponseWriter, r *http.Request) {
	alias := r.URL.Path[len("/api/servers/"):]
	if alias == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var update serverUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		server, ok := store.Servers[alias]
		if !ok {
			jsonError(w, fmt.Sprintf("server %q not found", alias), http.StatusNotFound)
			return
		}

		server.Alias = alias
		server.Host = update.Host
		server.Username = update.Username
		if update.Password != nil {
			server.Password = *update.Password
		}
		if update.KeyPath != nil {
			server.KeyPath = *update.KeyPath
			if strings.TrimSpace(*update.KeyPath) == "" {
				server.KeyPass = ""
			}
		}
		if update.KeyPass != nil {
			server.KeyPass = *update.KeyPass
		}
		server = config.NormalizeServer(server)
		if err := config.ValidateServer(server); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		store.Upsert(server)
		if err := store.Save(); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, publicServer(server))

	case http.MethodDelete:
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := store.Remove(alias); err != nil {
			jsonError(w, fmt.Sprintf("server %q not found", alias), http.StatusNotFound)
			return
		}
		if err := store.Save(); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSHKeys handles GET /api/sshkeys.
// It scans the user's ~/.ssh directory (one level deep) for private key files
// so the web UI can offer them as a picker instead of manual path typing.
func handleSSHKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keys, sshDir := findSSHKeys()
	jsonOK(w, map[string]any{
		"ssh_dir": sshDir,
		"keys":    keys,
	})
}

// handleSSHKeySelect handles POST /api/sshkeys/select.
// Browsers only expose a fake path (`C:\fakepath\...`) for a native file
// input, so the real OS file-open dialog runs on the machine that hosts the
// gscp server — the normal `gscp serve` setup. The handler blocks until the
// user picks a file or cancels, then returns the chosen absolute path.
func handleSSHKeySelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		jsonError(w, "文件选择器只能在服务器本机使用", http.StatusForbidden)
		return
	}
	path, err := showFilePickerForSSHKey()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"path": path})
}

// isLoopbackRequest reports whether the request came from the loopback
// interface, i.e. the browser runs on the same machine as the server.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sshKeyDialogTimeout bounds how long a native SSH key file dialog may stay
// open. The web server's write timeout must be at least as generous so the
// request is not cut off while the user browses for a file.
const sshKeyDialogTimeout = 10 * time.Minute

// showFilePickerForSSHKey opens a native file-open dialog on the machine
// running the server, pre-positioned in ~/.ssh when it exists, and returns
// the chosen absolute path. An empty string means the user cancelled.
func showFilePickerForSSHKey() (string, error) {
	home, _ := os.UserHomeDir()
	startDir := filepath.Join(home, ".ssh")

	ctx, cancel := context.WithTimeout(context.Background(), sshKeyDialogTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", windowsSSHKeyPickerScript(startDir))
	case "darwin":
		cmd = exec.CommandContext(ctx, "osascript", "-e", macSSHKeyPickerScript(startDir))
	default:
		cmd = exec.CommandContext(ctx, "zenity", "--file-selection", "--title=选择 SSH 私钥文件", "--filename="+startDir)
	}

	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("文件选择超时")
	}
	return pickerOutput(out, err)
}

// pickerOutput turns a native picker's output into a result: the chosen
// path, "" for a user cancel, or an error when the helper could not run.
func pickerOutput(out []byte, err error) (string, error) {
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// A non-zero exit with no output (or a "cancel" message) means the user
	// dismissed the dialog (zenity exits 1, osascript reports "User
	// canceled"). Any other error means the helper itself failed to run.
	if ee, ok := err.(*exec.ExitError); ok && strings.TrimSpace(string(out)) == "" {
		stderr := ""
		if ee.Stderr != nil {
			stderr = string(ee.Stderr)
		}
		if strings.TrimSpace(stderr) == "" || strings.Contains(strings.ToLower(stderr), "cancel") {
			return "", nil
		}
	}
	return "", fmt.Errorf("无法打开文件选择器: %v", err)
}

// windowsSSHKeyPickerScript returns the PowerShell script that shows a
// Windows OpenFileDialog and prints the chosen path. `powershell -STA` is
// required for WinForms dialogs to work.
func windowsSSHKeyPickerScript(startDir string) string {
	dir := strings.ReplaceAll(startDir, "'", "''")
	return fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '选择 SSH 私钥文件'
$d.Filter = 'SSH 私钥文件 (*.pem;*.key;*.p8;*)|*.pem;*.key;*.p8;*|所有文件 (*.*)|*.*'
$d.Multiselect = $false
if (Test-Path -LiteralPath '%s') { $d.InitialDirectory = '%s' }
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileName }
`, dir, dir)
}

// macSSHKeyPickerScript returns the AppleScript that shows an open-file
// dialog and prints the chosen POSIX path.
func macSSHKeyPickerScript(startDir string) string {
	dir := strings.ReplaceAll(startDir, `"`, `\"`)
	return fmt.Sprintf(`try
	set d to (POSIX file "%s") as alias
	set f to choose file with prompt "选择 SSH 私钥文件" default location d
on error
	set f to choose file with prompt "选择 SSH 私钥文件"
end try
POSIX path of f`, dir)
}

// knownNonKeyFiles are common ~/.ssh entries that are not private keys.
var knownNonKeyFiles = map[string]bool{
	"config":      true,
	"environment": true,
	"rc":          true,
	"ssh_config":  true,
	"readme":      true,
	"password":    true,
}

// isSSHKeyFile reports whether a file entry is plausibly a private key file.
func isSSHKeyFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	if knownNonKeyFiles[lower] {
		return false
	}
	// authorized_keys* and known_hosts* — including rotated backups such as
	// known_hosts.old — are verification/host files, never private keys.
	if strings.HasPrefix(lower, "authorized_keys") || strings.HasPrefix(lower, "known_hosts") {
		return false
	}
	if strings.HasSuffix(lower, ".pub") {
		return false
	}
	if strings.HasSuffix(lower, ".ppk") {
		return false
	}
	if strings.HasPrefix(lower, "ssh_host_") {
		return false
	}
	return true
}

// findSSHKeys returns absolute paths of private key files under ~/.ssh
// (including subdirectories such as ~/.ssh/keys/), sorted, together with
// the .ssh directory itself. An empty result is returned when the home
// directory or ~/.ssh cannot be determined.
func findSSHKeys() (keys []string, sshDir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{}, ""
	}
	sshDir = filepath.Join(home, ".ssh")
	if _, err := os.Stat(sshDir); err != nil {
		return []string{}, sshDir
	}
	_ = filepath.WalkDir(sshDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			// Descend into subdirectories so ~/.ssh/keys/gcp.pem is found.
			return nil
		}
		if isSSHKeyFile(d.Name()) {
			keys = append(keys, path)
		}
		return nil
	})
	sort.Strings(keys)
	if keys == nil {
		keys = []string{}
	}
	return keys, sshDir
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleWorkspaces handles GET /api/workspaces and DELETE /api/workspaces
func handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		workspaces := store.Workspaces
		if workspaces == nil {
			workspaces = []string{}
		}

		type WorkspaceInfo struct {
			Path      string `json:"path"`
			GitBranch string `json:"git_branch,omitempty"`
		}

		result := make([]WorkspaceInfo, len(workspaces))
		for i, path := range workspaces {
			result[i] = WorkspaceInfo{
				Path:      path,
				GitBranch: getGitBranch(path),
			}
		}

		jsonOK(w, result)

	case http.MethodDelete:
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
			jsonError(w, "path is required", http.StatusBadRequest)
			return
		}
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		filtered := store.Workspaces[:0]
		for _, wp := range store.Workspaces {
			if wp != body.Path {
				filtered = append(filtered, wp)
			}
		}
		store.Workspaces = filtered
		if err := store.Save(); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGenvRead handles POST /api/genv/read
// Body: {"path": "/abs/path/to/dir"}
// Returns the parsed .genv as JSON, plus a "raw" field with the original text.
func handleGenvRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		jsonError(w, "path is required", http.StatusBadRequest)
		return
	}

	genvPath := filepath.Join(body.Path, runconfig.FileName)
	data, err := os.ReadFile(genvPath)
	if os.IsNotExist(err) {
		jsonError(w, ".genv not found", http.StatusNotFound)
		return
	}
	if err != nil {
		jsonError(w, "read .genv: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cfg, _, err := runconfig.LoadConfigFromDir(body.Path)
	if err != nil {
		jsonError(w, "parse .genv: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	type response struct {
		Path    string                      `json:"path"`
		Groups  map[string][]string         `json:"groups"`
		Targets map[string]runconfig.Target `json:"targets"`
		Raw     string                      `json:"raw"`
	}
	groups := cfg.Groups
	if groups == nil {
		groups = map[string][]string{}
	}
	targets := cfg.Targets
	if targets == nil {
		targets = map[string]runconfig.Target{}
	}
	jsonOK(w, response{
		Path:    body.Path,
		Groups:  groups,
		Targets: targets,
		Raw:     string(data),
	})
}

// handleGenvWrite handles POST /api/genv/write
// Body: {"path": "/abs/path/to/dir", "raw": "<json string>"}
// Writes the raw JSON string directly to .genv (after validating it parses).
func handleGenvWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
		Raw  string `json:"raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" || body.Raw == "" {
		jsonError(w, "path and raw are required", http.StatusBadRequest)
		return
	}

	// Validate the JSON is parseable as a .genv before writing.
	var check map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body.Raw), &check); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	genvPath := filepath.Join(body.Path, runconfig.FileName)
	if err := os.WriteFile(genvPath, []byte(body.Raw), 0o644); err != nil {
		jsonError(w, "write .genv: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleWorkspaceAdd handles POST /api/workspaces/add
// Body: {"path": "/abs/path/to/dir"}
// Adds the path to the workspace list if not already present.
func handleWorkspaceAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		jsonError(w, "path is required", http.StatusBadRequest)
		return
	}
	store, err := config.Load()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	store.AddWorkspace(body.Path)
	if err := store.Save(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleScan handles GET /api/scan using Server-Sent Events.
// It streams three event types:
//   - "scanning" : {"dir": "<current dir being entered>"}
//   - "found"    : {"path": "<dir containing .genv>"}
//   - "done"     : {"count": N}
func handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sendEvent := func(event, dataJSON string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, dataJSON)
		flusher.Flush()
	}

	store, err := config.Load()
	if err != nil {
		sendEvent("error", `{"error":"`+err.Error()+`"}`)
		return
	}
	ss := store.GetScanSettings()

	skipSet := make(map[string]struct{}, len(ss.SkipDirs))
	for _, d := range ss.SkipDirs {
		skipSet[d] = struct{}{}
	}

	roots := ss.ScanRoots
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			sendEvent("error", `{"error":"cannot determine home dir"}`)
			return
		}
		roots = []string{home}
	}

	ctx := r.Context()
	seen := make(map[string]struct{})
	found := 0

	// Throttle "scanning" events: only send when dir changes at depth ≤ 3
	// to avoid flooding the client with thousands of messages.
	lastSent := ""

	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			// Respect client disconnect
			select {
			case <-ctx.Done():
				return fmt.Errorf("client disconnected")
			default:
			}

			if err != nil {
				return filepath.SkipDir
			}

			if d.IsDir() {
				name := d.Name()
				if path != root {
					if _, skip := skipSet[name]; skip {
						return filepath.SkipDir
					}
					if len(name) > 1 && name[0] == '.' {
						return filepath.SkipDir
					}
				}
				// Send progress for shallow dirs to give feedback without flooding
				rel, _ := filepath.Rel(root, path)
				depth := 0
				for _, c := range rel {
					if c == os.PathSeparator {
						depth++
					}
				}
				if depth <= 2 && path != lastSent {
					lastSent = path
					b, _ := json.Marshal(map[string]string{"dir": path})
					sendEvent("scanning", string(b))
				}
				return nil
			}

			if d.Name() == runconfig.FileName {
				dir := filepath.Dir(path)
				if _, ok := seen[dir]; !ok {
					seen[dir] = struct{}{}
					found++
					b, _ := json.Marshal(map[string]string{"path": dir})
					sendEvent("found", string(b))
				}
			}
			return nil
		})
	}

	b, _ := json.Marshal(map[string]int{"count": found})
	sendEvent("done", string(b))
}

// handleSettings handles GET /api/settings and PUT /api/settings
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ss := store.GetScanSettings()
		jsonOK(w, ss)

	case http.MethodPut:
		var ss config.ScanSettings
		if err := json.NewDecoder(r.Body).Decode(&ss); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		store.ScanSettings = &ss
		if err := store.Save(); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, ss)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
