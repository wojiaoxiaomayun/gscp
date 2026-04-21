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
	mux.HandleFunc("/api/servers", handleServers)
	mux.HandleFunc("/api/servers/", handleServerByAlias)
	mux.HandleFunc("/api/workspaces", handleWorkspaces)
	mux.HandleFunc("/api/workspaces/add", handleWorkspaceAdd)
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)
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
		WriteTimeout: 60 * time.Second,
	}

	return srv.Serve(ln)
}

// RunWithContext starts the web server and shuts it down when ctx is cancelled.
func RunWithContext(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/servers", handleServers)
	mux.HandleFunc("/api/servers/", handleServerByAlias)
	mux.HandleFunc("/api/workspaces", handleWorkspaces)
	mux.HandleFunc("/api/workspaces/add", handleWorkspaceAdd)
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)
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
		WriteTimeout: 60 * time.Second,
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

// handleServers handles GET /api/servers and POST /api/servers
func handleServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		store, err := config.Load()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, store.List())

	case http.MethodPost:
		var server config.Server
		if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if server.Alias == "" || server.Host == "" || server.Username == "" || server.Password == "" {
			jsonError(w, "alias, host, username and password are required", http.StatusBadRequest)
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
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, server)

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
		var server config.Server
		if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		server.Alias = alias
		if server.Host == "" || server.Username == "" || server.Password == "" {
			jsonError(w, "host, username and password are required", http.StatusBadRequest)
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
		jsonOK(w, server)

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
