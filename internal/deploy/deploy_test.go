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
