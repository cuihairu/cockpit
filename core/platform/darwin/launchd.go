//go:build darwin

// Package darwin macOS 平台适配（agent-core ②）：机器标识/launchd 服务交互。
// 平台分支只存在于本目录。
package darwin

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ============ macOS launchd 交互层（service-design.md D10，P4 平台收编）============
//
// 本文件只做 OS 交互（launchctl argv 直调、LaunchDaemons 目录枚举读取）；
// plist/launchctl list 解析、ServiceUnit 映射与 label/动词校验留在 rpc 层
// （无 build tag，Linux CI 可测）。只管 system 域（agent 以 root 运行前提
// 与 systemd 侧同）。

// DaemonDirs 扫描的 LaunchDaemons 目录（D10.1：不含用户域 LaunchAgents）
var DaemonDirs = []string{
	"/Library/LaunchDaemons",
	"/System/Library/LaunchDaemons",
}

// Run launchctl argv 直调（超时由调用方 ctx 控制），返回原始 stdout/stderr
// 三元组与 Commander 同形，错误摘要组装交给调用方
func Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Plist 一个已读取的 LaunchDaemons plist 文件
type Plist struct {
	Path string // 文件路径（→ ServiceUnit.Description）
	Data []byte // 原始内容（解析在调用方）
}

// DaemonPlists 枚举 DaemonDirs 下 *.plist 并读取内容：目录不存在忽略
// （防御 /System 缺失的非常规环境），单文件读取失败跳过（log）
func DaemonPlists() []Plist {
	var out []Plist
	for _, dir := range DaemonDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("launchd scan: read %s: %v", path, err)
				continue
			}
			out = append(out, Plist{Path: path, Data: data})
		}
	}
	return out
}
