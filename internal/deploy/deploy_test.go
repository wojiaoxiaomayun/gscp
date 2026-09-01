package deploy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gscp/internal/config"

	"golang.org/x/crypto/ssh"
)

func writeTestPrivateKey(t *testing.T, encrypted bool, passphrase string) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}

	var block *pem.Block
	if encrypted {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "gscp test", []byte(passphrase))
	} else {
		block, err = ssh.MarshalPrivateKey(privateKey, "gscp test")
	}
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return path
}

func TestLoadPrivateKey(t *testing.T) {
	plainPath := writeTestPrivateKey(t, false, "")
	encryptedPath := writeTestPrivateKey(t, true, "correct passphrase")

	tests := []struct {
		name       string
		path       string
		passphrase string
		wantErr    string
	}{
		{name: "plain", path: plainPath},
		{name: "plain with extra passphrase", path: plainPath, passphrase: "unused"},
		{name: "encrypted", path: encryptedPath, passphrase: "correct passphrase"},
		{name: "encrypted missing passphrase", path: encryptedPath, wantErr: "key is encrypted but key_pass is empty"},
		{name: "encrypted wrong passphrase", path: encryptedPath, passphrase: "wrong", wantErr: "parse encrypted private key"},
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing"), wantErr: "read private key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadPrivateKey(tt.path, tt.passphrase)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("load private key: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestBuildAuthMethodsKeepsPasswordFallback(t *testing.T) {
	server := config.Server{
		Password: "fallback password",
		KeyPath:  filepath.Join(t.TempDir(), "missing-key"),
	}

	methods, keyErr := buildAuthMethods(server)
	if keyErr == nil || !strings.Contains(keyErr.Error(), "read private key") {
		t.Fatalf("expected private key diagnostic, got %v", keyErr)
	}
	if len(methods) != 1 {
		t.Fatalf("expected password fallback auth method, got %d methods", len(methods))
	}
}

func TestBuildAuthMethodsUsesKeyBeforePassword(t *testing.T) {
	server := config.Server{
		Password: "fallback password",
		KeyPath:  writeTestPrivateKey(t, false, ""),
	}

	methods, keyErr := buildAuthMethods(server)
	if keyErr != nil {
		t.Fatalf("build auth methods: %v", keyErr)
	}
	if len(methods) != 2 {
		t.Fatalf("expected key and password auth methods, got %d", len(methods))
	}
}

func TestExpandHomeSupportsSlashStyles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, input := range []string{"~/.ssh/id_ed25519", `~\.ssh\id_ed25519`} {
		got := expandHome(input)
		if !strings.HasPrefix(got, home) {
			t.Fatalf("expected %q to expand beneath %q, got %q", input, home, got)
		}
	}
}

func TestIgnoreMatcher(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		rel      string
		want     bool
	}{
		// Names without "/" match at any depth.
		{name: "plain name matches base", patterns: []string{"static"}, rel: "static", want: true},
		{name: "plain name matches nested", patterns: []string{"static"}, rel: "dist/static", want: true},
		{name: "plain name matches deep nested", patterns: []string{"static"}, rel: "a/b/c/static", want: true},
		{name: "plain name does not match other", patterns: []string{"static"}, rel: "dist", want: false},
		{name: "plain name does not match partial", patterns: []string{"static"}, rel: "static2", want: false},
		// Glob names without "/".
		{name: "star glob matches basename", patterns: []string{"*.map"}, rel: "js/app.js.map", want: true},
		{name: "star glob any depth", patterns: []string{"*.map"}, rel: "app.js.map", want: true},
		{name: "star glob no match", patterns: []string{"*.map"}, rel: "app.js", want: false},
		{name: "question glob", patterns: []string{"a?"}, rel: "ab", want: true},
		{name: "question glob no match", patterns: []string{"a?"}, rel: "a", want: false},
		{name: "class glob", patterns: []string{"[0-9].txt"}, rel: "7.txt", want: true},
		// Anchored patterns with "/".
		{name: "anchored exact", patterns: []string{"dist/static"}, rel: "dist/static", want: true},
		{name: "anchored exact dir not ancestor", patterns: []string{"dist/static"}, rel: "dist", want: false},
		{name: "anchored exact dir not child", patterns: []string{"dist/static"}, rel: "dist/static/img/x.png", want: false},
		{name: "anchored substring rel dist", patterns: []string{"dist/static"}, rel: "x/dist/static", want: false},
		{name: "double star middle", patterns: []string{"assets/**/cache"}, rel: "assets/cache", want: true},
		{name: "double star middle nested", patterns: []string{"assets/**/cache"}, rel: "assets/a/b/cache", want: true},
		{name: "double star middle no match", patterns: []string{"assets/**/cache"}, rel: "cache", want: false},
		{name: "leading double star", patterns: []string{"**/cache"}, rel: "a/b/cache", want: true},
		{name: "leading double star root", patterns: []string{"**/cache"}, rel: "cache", want: true},
		{name: "trailing double star", patterns: []string{"dist/**"}, rel: "dist", want: true},
		{name: "trailing double star nested", patterns: []string{"dist/**"}, rel: "dist/a/b", want: true},
		{name: "single star segment", patterns: []string{"dist/*"}, rel: "dist/a", want: true},
		{name: "single star segment no slash", patterns: []string{"dist/*"}, rel: "dist/a/b", want: false},
		{name: "dot relative prefix", patterns: []string{"./dist/static"}, rel: "dist/static", want: true},
		{name: "trailing slash dir only", patterns: []string{"dist/static/"}, rel: "dist/static", want: true},
		{name: "empty and blank patterns", patterns: []string{"", "   "}, rel: "a", want: false},
		// Windows style backslashes normalized.
		{name: "backslash pattern", patterns: []string{`dist\static`}, rel: "dist/static", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newIgnoreMatcher(tt.patterns)
			if got := m.match(tt.rel); got != tt.want {
				t.Fatalf("match(%q) with %v = %v, want %v", tt.rel, tt.patterns, got, tt.want)
			}
		})
	}
}

// makeFixtureTree creates a directory tree like:
//
//	dist/index.html
//	dist/app.js
//	dist/static/img/logo.png
//	dist/js/vendor.js.map
//	dist/skipme.txt
func makeFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := []string{
		"dist/index.html",
		"dist/app.js",
		"dist/static/img/logo.png",
		"dist/static/img/favicon.ico",
		"dist/js/vendor.js.map",
		"dist/js/app.js.map",
		"dist/skipme.txt",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return filepath.Join(root, "dist")
}

func remoteNames(t *testing.T, items []uploadItem) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for _, it := range items {
		rel, ok := strings.CutPrefix(it.RemotePath, "/srv/www/")
		if !ok {
			t.Fatalf("unexpected remote path %q", it.RemotePath)
		}
		got[rel] = true
	}
	return got
}

func TestBuildUploadPlanIgnoreDir(t *testing.T) {
	local := makeFixtureTree(t)
	info, err := os.Stat(local)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	items, err := buildUploadPlan(local, "/srv/www", info, []string{"static"})
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}

	got := remoteNames(t, items)
	want := map[string]bool{
		"index.html":         true,
		"app.js":             true,
		"js/vendor.js.map":   true,
		"js/app.js.map":      true,
		"skipme.txt":         true,
	}
	if len(got) != len(want) {
		t.Fatalf("item count = %d, want %d (%v): got %v", len(got), len(want), want, got)
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("missing %q in %v", name, got)
		}
	}
}

func TestBuildUploadPlanIgnoreGlobs(t *testing.T) {
	local := makeFixtureTree(t)
	info, err := os.Stat(local)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	items, err := buildUploadPlan(local, "/srv/www", info, []string{"*.map", "skip?e.txt"})
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}

	got := remoteNames(t, items)
	if _, ok := got["js/vendor.js.map"]; ok {
		t.Fatalf("*.map should have excluded js/vendor.js.map, got %v", got)
	}
	if _, ok := got["js/app.js.map"]; ok {
		t.Fatalf("*.map should have excluded js/app.js.map, got %v", got)
	}
	if _, ok := got["skipme.txt"]; ok {
		t.Fatalf("skip?e.txt should have excluded skipme.txt, got %v", got)
	}
	if !got["index.html"] || !got["app.js"] {
		t.Fatalf("non-matching files missing, got %v", got)
	}
}

func TestBuildUploadPlanIgnoreAnchored(t *testing.T) {
	local := makeFixtureTree(t)
	info, err := os.Stat(local)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	items, err := buildUploadPlan(local, "/srv/www", info, []string{"static/**", "js/app.js.map"})
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}

	got := remoteNames(t, items)
	for name := range got {
		if strings.HasPrefix(name, "static/") || name == "static" {
			t.Fatalf("static subtree should be excluded, got %q in %v", name, got)
		}
	}
	if _, ok := got["js/app.js.map"]; ok {
		t.Fatalf("js/app.js.map should be excluded, got %v", got)
	}
	if !got["js/vendor.js.map"] {
		t.Fatalf("js/vendor.js.map should remain, got %v", got)
	}
	if !got["index.html"] {
		t.Fatalf("index.html should remain, got %v", got)
	}
}

func TestBuildUploadPlanIgnoreNonDirRoot(t *testing.T) {
	local := filepath.Join(makeFixtureTree(t), "index.html")
	info, err := os.Stat(local)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	// Matched by name -> nothing uploaded.
	items, err := buildUploadPlan(local, "/srv/www", info, []string{"index.html"})
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for matched file root, got %v", items)
	}

	// Unmatched -> single file uploaded.
	items, err = buildUploadPlan(local, "/srv/www", info, []string{"other"})
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}
	if len(items) != 1 || filepath.Base(items[0].LocalPath) != "index.html" {
		t.Fatalf("expected single uploaded file, got %v", items)
	}
}

func TestBuildUploadPlanNoIgnore(t *testing.T) {
	local := makeFixtureTree(t)
	info, err := os.Stat(local)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	items, err := buildUploadPlan(local, "/srv/www", info, nil)
	if err != nil {
		t.Fatalf("build upload plan: %v", err)
	}
	if len(items) != 7 {
		t.Fatalf("expected 7 items without ignore, got %d", len(items))
	}
}
