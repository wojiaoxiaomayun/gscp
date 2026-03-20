package cli

import (
	"reflect"
	"testing"

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
