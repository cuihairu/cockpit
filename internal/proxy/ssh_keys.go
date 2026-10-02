package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// 默认私钥文件名（按优先级排列）
var defaultKeyNames = []string{
	"id_ed25519",
	"id_rsa",
	"id_ecdsa",
	"id_dsa",
}

// LoadDefaultSSHKeys 从 keyDir 加载默认 SSH 私钥。
// 扫描 defaultKeyNames 列表，返回所有成功解析的 AuthMethod。
// keyDir 为空时使用 ~/.ssh/。
func LoadDefaultSSHKeys(keyDir string) ([]ssh.AuthMethod, error) {
	if keyDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		keyDir = filepath.Join(home, ".ssh")
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
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		keyDir = filepath.Join(home, ".ssh")
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
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		keyDir = filepath.Join(home, ".ssh")
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