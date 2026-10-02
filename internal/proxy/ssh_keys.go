package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// 覆盖率注入点（同 agent.go goos 范式）：生产实现对内存操作恒成功
// （ed25519 密钥生成/PEM 序列化不会失败），错误分支仅供测试注入覆盖。
var (
	generateEd25519Key = ed25519.GenerateKey
	marshalPrivateKey  = ssh.MarshalPrivateKey
)

// EnsureSSHKeys 确保 keyDir 下有可用的 SSH 密钥。
// 如果不存在，自动生成 Ed25519 密钥对。返回使用的密钥路径。
func EnsureSSHKeys(keyDir string) (string, error) {
	if keyDir == "" {
		keyDir = defaultSSHDir()
	}

	// 已有可用密钥，直接返回
	for _, name := range defaultKeyNames {
		keyPath := filepath.Join(keyDir, name)
		if _, err := os.Stat(keyPath); err == nil {
			return keyPath, nil
		}
	}

	// 没有密钥，自动生成 Ed25519
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", fmt.Errorf("create ssh dir %s: %w", keyDir, err)
	}

	_, priv, err := generateEd25519Key(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate ed25519 key: %w", err)
	}

	// 序列化私钥为 PEM
	privDER, err := marshalPrivateKey(priv, "cockpit-agent@auto")
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(privDER)

	keyPath := filepath.Join(keyDir, "id_ed25519")
	if err := os.WriteFile(keyPath, privPEM, 0600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}

	// 写公钥
	pub, _ := ssh.NewPublicKey(priv.Public())
	pubLine := string(ssh.MarshalAuthorizedKey(pub))
	pubPath := keyPath + ".pub"
	if err := os.WriteFile(pubPath, []byte(pubLine), 0644); err != nil {
		return "", fmt.Errorf("write public key: %w", err)
	}

	log.Printf("Auto-generated SSH key: %s", keyPath)
	return keyPath, nil
}

// 默认私钥文件名（按优先级排列）
var defaultKeyNames = []string{
	"id_ed25519",
	"id_rsa",
	"id_ecdsa",
	"id_dsa",
}

// defaultSSHDir 返回默认 SSH 密钥目录。
// 优先 COCKPIT_SSH_KEYS 环境变量；其次实际登录用户（SUDO_USER）的 ~/.ssh/；
// 最后 fallback 到当前用户的 ~/.ssh/。
func defaultSSHDir() string {
	// 环境变量优先
	if dir := os.Getenv("COCKPIT_SSH_KEYS"); dir != "" {
		return dir
	}
	// sudo 运行时 SUDO_USER 指向实际用户
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := os.UserHomeDir(); err == nil {
			// /home/{SUDO_USER}/.ssh
			return filepath.Join(filepath.Dir(filepath.Dir(u)), sudoUser, ".ssh")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh")
}

// LoadDefaultSSHKeys 从 keyDir 加载默认 SSH 私钥。
// 扫描 defaultKeyNames 列表，返回所有成功解析的 AuthMethod。
// keyDir 为空时使用 defaultSSHDir()。
func LoadDefaultSSHKeys(keyDir string) ([]ssh.AuthMethod, error) {
	if keyDir == "" {
		keyDir = defaultSSHDir()
	}

	var methods []ssh.AuthMethod
	for _, name := range defaultKeyNames {
		keyPath := filepath.Join(keyDir, name)
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue // 文件不存在，跳过
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue // 解析失败（可能有密码保护），跳过
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	return methods, nil
}

// LoadSSHKeyFromFile 从指定路径加载单个 SSH 私钥。
func LoadSSHKeyFromFile(keyPath string) (ssh.AuthMethod, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", keyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse key %s: %w", keyPath, err)
	}
	return ssh.PublicKeys(signer), nil
}

// HasPrivateKeyFiles 检查 keyDir 下是否有可用的私钥文件。
func HasPrivateKeyFiles(keyDir string) bool {
	if keyDir == "" {
		keyDir = defaultSSHDir()
	}
	for _, name := range defaultKeyNames {
		path := filepath.Join(keyDir, name)
		if _, err := os.Stat(path); err == nil {
			// 检查对应的 .pub 文件也存在（确认是密钥对）
			pubPath := path + ".pub"
			if _, err := os.Stat(pubPath); err == nil {
				return true
			}
		}
	}
	return false
}

// ListSSHKeyNames 返回 keyDir 下可用的私钥文件名。
func ListSSHKeyNames(keyDir string) []string {
	if keyDir == "" {
		keyDir = defaultSSHDir()
	}

	entries, err := os.ReadDir(keyDir)
	if err != nil {
		return nil
	}

	var keys []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, ".pub") || strings.HasPrefix(name, ".") {
			continue
		}
		// 检查是否是私钥文件
		for _, def := range defaultKeyNames {
			if name == def {
				keys = append(keys, name)
				break
			}
		}
	}
	return keys
}
