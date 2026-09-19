package detector

import (
	"os"
	"path/filepath"
	"testing"
)

// overlayFakeBin 在临时目录造一个可执行文件并让 LookPath 命中
func overlayFakeBin(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	fp := filepath.Join(dir, name)
	if err := os.WriteFile(fp, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write %s: %v", fp, err)
	}
	// LookPath 需要目录在 PATH 中
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestOverlayDetectorNoneInstalled(t *testing.T) {
	covEmptyPATH(t)
	d := &OverlayDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap != nil {
		t.Errorf("capability = %+v, want nil when no tool installed", cap)
	}
}

func TestOverlayDetectorPartialTools(t *testing.T) {
	covEmptyPATH(t)
	overlayFakeBin(t, "zerotier-cli")
	overlayFakeBin(t, "frpc")

	d := &OverlayDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Type != "overlay" {
		t.Fatalf("capability = %+v, want type overlay", cap)
	}
	if cap.Metadata["zerotier"] != true || cap.Metadata["frp"] != true {
		t.Errorf("metadata = %v, want zerotier+frp", cap.Metadata)
	}
	if _, ok := cap.Metadata["tailscale"]; ok {
		t.Error("tailscale should not be detected")
	}
}

func TestOverlayDetectorFrpsOnly(t *testing.T) {
	covEmptyPATH(t)
	overlayFakeBin(t, "frps")
	d := &OverlayDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil || cap.Metadata["frp"] != true {
		t.Fatalf("capability = %+v, want frp via frps-only", cap)
	}
}

func TestOverlayDetectorMeta(t *testing.T) {
	d := &OverlayDetector{}
	if d.Name() != "overlay" {
		t.Errorf("Name() = %q", d.Name())
	}
	if d.Priority() <= 15 {
		t.Errorf("Priority() = %d, want 16", d.Priority())
	}
}
