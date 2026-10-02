package rpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// passwdPath 覆盖率注入点（同 internal/agent agent.go goos 范式）：默认
// 生产路径，同包测试注入不存在路径覆盖「读不到 passwd → 兜底 root」分支，
// 注入前后 defer 恢复。读方仅 inferUsername（RPC 测试同步调用）。
var passwdPath = "/etc/passwd"

// SSHProvider 远程 SSH 密钥管理 Provider。
// 提供 getDefaultKey action：返回默认 SSH 私钥 PEM，供 guacd 认证。
type SSHProvider struct {
	keyDir string
}

func NewSSHProvider(keyDir string) *SSHProvider {
	return &SSHProvider{keyDir: keyDir}
}

func (p *SSHProvider) Type() string { return "ssh" }

func (p *SSHProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "getDefaultKey":
		return p.getDefaultKey()
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// getDefaultKey 返回默认 SSH 私钥 PEM 和对应的用户名。
func (p *SSHProvider) getDefaultKey() (interface{}, error) {
	dir := p.keyDir
	if dir == "" {
		dir = defaultSSHDir()
	}

	// 按优先级扫描
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"} {
		keyPath := filepath.Join(dir, name)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}
		// 验证是有效私钥
		if _, err := ssh.ParsePrivateKey(data); err != nil {
			continue
		}
		// 推断用户名：优先用 SUDO_USER，其次 root
		username := inferUsername()
		return map[string]interface{}{
			"privateKey": string(data),
			"username":   username,
			"keyFile":    name,
		}, nil
	}

	return nil, fmt.Errorf("no usable SSH private key found in %s", dir)
}

func defaultSSHDir() string {
	if dir := os.Getenv("COCKPIT_SSH_KEYS"); dir != "" {
		return dir
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(filepath.Dir(filepath.Dir(home)), sudoUser, ".ssh")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh")
}

func inferUsername() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	// 读 /etc/passwd 的当前 UID（passwdPath 见包级注入点注释）
	if out, err := os.ReadFile(passwdPath); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 && parts[2] == "0" {
				return parts[0]
			}
		}
	}
	return "root"
}
