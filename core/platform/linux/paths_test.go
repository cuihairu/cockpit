//go:build linux

package linux

import "testing"

// TestPathDirs 目录三元组非空且沿 install.sh 家族（数据面与 unit
// ReadWritePaths 同径）。
func TestPathDirs(t *testing.T) {
	c, d, l := PathDirs()
	if c != "/etc/cockpit-agent" || d != "/var/lib/cockpit-agent" || l != "/var/log/cockpit-agent" {
		t.Fatalf("PathDirs() = %q, %q, %q", c, d, l)
	}
}

// TestGracefulSignals unix 优雅退出全集：SIGTERM（systemd stop）+ SIGINT
// （前台 Ctrl+C）。
func TestGracefulSignals(t *testing.T) {
	sig := GracefulSignals()
	if len(sig) != 2 {
		t.Fatalf("GracefulSignals() len = %d, want 2", len(sig))
	}
}
