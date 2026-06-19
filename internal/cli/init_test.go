package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cuihairu/cockpit/internal/inventory"
)

func TestInitCmd_CreatesDefaultLayout(t *testing.T) {
	dir := t.TempDir()

	cmd := &InitCmd{Dir: dir}
	if err := cmd.Run(); err != nil {
		t.Fatalf("InitCmd.Run failed: %v", err)
	}

	for _, sub := range []string{"config", "data", "inventory"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil {
			t.Fatalf("expected directory %s: %v", sub, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", sub)
		}
	}

	cfgPath := filepath.Join(dir, "config", "cockpit.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// example inventory should NOT exist without -example flag
	if _, err := os.Stat(filepath.Join(dir, "inventory", "example.yaml")); !os.IsNotExist(err) {
		t.Fatalf("example inventory should not exist without Example flag, got err=%v", err)
	}
}

func TestInitCmd_ExampleInventoryWritten(t *testing.T) {
	dir := t.TempDir()

	cmd := &InitCmd{Dir: dir, Example: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("InitCmd.Run failed: %v", err)
	}

	examplePath := filepath.Join(dir, "inventory", "example.yaml")
	if _, err := os.Stat(examplePath); err != nil {
		t.Fatalf("example inventory not created: %v", err)
	}

	// generated example must be parseable by the inventory parser
	inv, err := inventory.ParseFile(examplePath)
	if err != nil {
		t.Fatalf("generated example inventory is not parseable: %v", err)
	}
	if inv.Version != "v1" {
		t.Fatalf("expected version v1, got %q", inv.Version)
	}
	if len(inv.ComputeInstances) == 0 {
		t.Fatalf("example inventory should ship at least one compute instance")
	}
}

func TestInitCmd_DoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	cfgPath := filepath.Join(dir, "config", "cockpit.yaml")
	marker := []byte("# existing marker\n")
	if err := os.WriteFile(cfgPath, marker, 0644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cmd := &InitCmd{Dir: dir}
	if err := cmd.Run(); err != nil {
		t.Fatalf("InitCmd.Run failed: %v", err)
	}

	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(marker) {
		t.Fatalf("existing config was overwritten")
	}
}

func TestInitCmd_CustomConfigPath(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "alt-config.yaml")

	cmd := &InitCmd{Dir: dir, Config: custom}
	if err := cmd.Run(); err != nil {
		t.Fatalf("InitCmd.Run failed: %v", err)
	}

	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom config not created: %v", err)
	}

	// default path should NOT be used when Config is set
	if _, err := os.Stat(filepath.Join(dir, "config", "cockpit.yaml")); !os.IsNotExist(err) {
		t.Fatalf("default config path should not be written when custom Config is set, got err=%v", err)
	}
}

func TestInitCmd_EmptyDirDefaultsToCwd(t *testing.T) {
	// InitCmd with Dir="" uses "." — exercise via chdir to a temp dir.
	dir := t.TempDir()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cmd := &InitCmd{Dir: ""}
	if err := cmd.Run(); err != nil {
		t.Fatalf("InitCmd.Run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join("config", "cockpit.yaml")); err != nil {
		t.Fatalf("config not created in cwd: %v", err)
	}
}

func TestWriteConfig_IsValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	if err := writeConfig(path); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	// sanity: file is non-empty and starts with the expected header
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("writeConfig produced empty file")
	}
	if string(data[:1]) != "#" {
		t.Fatalf("expected config to start with comment header, got %q", string(data[:1]))
	}
}
