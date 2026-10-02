package rpc

// ssh_provider_test.go 覆盖 SSH 密钥管理 Provider 的全部分支：
// getDefaultKey 的有效/无效/缺失密钥、keyDir 为空时的 defaultSSHDir
// 三分支、inferUsername 的 SUDO_USER/USER/passwd/兜底四分支（注入点见
// ssh_provider.go 注释）。

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// genTestKeyPEM 生成一把可用作 SSH 私钥文件的 ed25519 PEM
func genTestKeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

// TestSSHProviderTypeAndUnknownAction 覆盖构造、Type 与 Call 的未知 action
func TestSSHProviderTypeAndUnknownAction(t *testing.T) {
	p := NewSSHProvider(t.TempDir())
	if p.Type() != "ssh" {
		t.Errorf("Type() = %q, want ssh", p.Type())
	}
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("Call(bogus) = %v, want unknown action error", err)
	}
}

// TestSSHProviderGetDefaultKeyNoKey 覆盖目录里没有任何密钥 → 扫描完报错
func TestSSHProviderGetDefaultKeyNoKey(t *testing.T) {
	p := NewSSHProvider(t.TempDir())
	_, err := p.Call("getDefaultKey", nil)
	if err == nil || !strings.Contains(err.Error(), "no usable SSH private key") {
		t.Errorf("getDefaultKey() = %v, want no usable key error", err)
	}
}

// TestSSHProviderGetDefaultKeyInvalidThenValid 覆盖：文件在但不是合法私钥
// （ParsePrivateKey 失败 continue），随后下一个候选名是合法私钥 → 命中
func TestSSHProviderGetDefaultKeyInvalidThenValid(t *testing.T) {
	dir := t.TempDir()
	// id_ed25519 是最高优先候选，但内容是垃圾：ParsePrivateKey 失败应跳过
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("not a private key"), 0600); err != nil {
		t.Fatal(err)
	}
	// id_rsa 是次优先候选，写入合法密钥
	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), genTestKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SUDO_USER", "")
	t.Setenv("USER", "covuser")

	p := NewSSHProvider(dir)
	res, err := p.Call("getDefaultKey", nil)
	if err != nil {
		t.Fatalf("getDefaultKey() error = %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T, want map", res)
	}
	if m["keyFile"] != "id_rsa" {
		t.Errorf("keyFile = %v, want id_rsa (invalid id_ed25519 must be skipped)", m["keyFile"])
	}
	if m["username"] != "covuser" {
		t.Errorf("username = %v, want covuser", m["username"])
	}
	if pem, _ := m["privateKey"].(string); !strings.Contains(pem, "PRIVATE KEY") {
		t.Errorf("privateKey = %q, want PEM content", pem)
	}
}

// TestSSHProviderDefaultDirFromEnv 覆盖 keyDir 为空时 defaultSSHDir 的
// COCKPIT_SSH_KEYS 命中分支，getDefaultKey 拿到合法密钥
func TestSSHProviderDefaultDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), genTestKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COCKPIT_SSH_KEYS", dir)

	t.Setenv("SUDO_USER", "")
	t.Setenv("USER", "")

	p := NewSSHProvider("") // keyDir 为空 → defaultSSHDir()
	res, err := p.Call("getDefaultKey", nil)
	if err != nil {
		t.Fatalf("getDefaultKey() error = %v", err)
	}
	if m, _ := res.(map[string]interface{}); m["keyFile"] != "id_ed25519" {
		t.Errorf("keyFile = %v, want id_ed25519", m["keyFile"])
	}
}

// TestDefaultSSHDirBranches 覆盖 defaultSSHDir 三分支：
// COCKPIT_SSH_KEYS 命中 / SUDO_USER+HOME 拼接 / 兜底 UserHomeDir
func TestDefaultSSHDirBranches(t *testing.T) {
	t.Setenv("COCKPIT_SSH_KEYS", "/custom/keys")
	if got := defaultSSHDir(); got != "/custom/keys" {
		t.Errorf("env dir = %q, want /custom/keys", got)
	}

	t.Setenv("COCKPIT_SSH_KEYS", "")
	t.Setenv("SUDO_USER", "alice")
	// sudo 场景 HOME 已切到目标用户（如 /root），目录推导取两级父目录
	// 再拼 SUDO_USER 的 .ssh（ssh_provider.go 现行语义）
	t.Setenv("HOME", "/root")
	if got := defaultSSHDir(); got != "/alice/.ssh" {
		t.Errorf("sudo dir = %q, want /alice/.ssh", got)
	}

	t.Setenv("SUDO_USER", "")
	home, _ := os.UserHomeDir()
	if got := defaultSSHDir(); got != filepath.Join(home, ".ssh") {
		t.Errorf("fallback dir = %q, want %q", got, filepath.Join(home, ".ssh"))
	}
}

// TestInferUsernameBranches 覆盖 inferUsername 的优先级链：
// SUDO_USER > USER > /etc/passwd 的 UID0 行 > 兜底 root（passwdPath 注入点）
func TestInferUsernameBranches(t *testing.T) {
	t.Setenv("SUDO_USER", "sudoer")
	t.Setenv("USER", "plainuser")
	if got := inferUsername(); got != "sudoer" {
		t.Errorf("SUDO_USER = %q, want sudoer", got)
	}

	t.Setenv("SUDO_USER", "")
	if got := inferUsername(); got != "plainuser" {
		t.Errorf("USER = %q, want plainuser", got)
	}

	// 两者皆空 → /etc/passwd 首个 UID0 行（正常环境即 root）
	t.Setenv("USER", "")
	if got := inferUsername(); got != "root" {
		t.Errorf("passwd = %q, want root (first UID 0 entry)", got)
	}

	// passwd 读不到 → 兜底 root
	saved := passwdPath
	passwdPath = filepath.Join(t.TempDir(), "missing")
	t.Cleanup(func() { passwdPath = saved })
	if got := inferUsername(); got != "root" {
		t.Errorf("fallback = %q, want root", got)
	}
}
