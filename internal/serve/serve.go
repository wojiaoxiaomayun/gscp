package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)

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
		WriteTimeout: 15 * time.Second,
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
	mux.HandleFunc("/api/genv/read", handleGenvRead)
	mux.HandleFunc("/api/genv/write", handleGenvWrite)

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
		WriteTimeout: 15 * time.Second,
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
		jsonOK(w, workspaces)

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
