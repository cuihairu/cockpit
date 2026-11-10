// 通知收包、server 进程观测与调度时刻辅助（自 main.go 按职责拆分）。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ---------- 通知收包 ----------

type hookEntry struct {
	TS       string `json:"ts"`
	Path     string `json:"path"`
	SecretOK bool   `json:"secret_ok"`
	Body     string `json:"body"`
}

func readJSONL(path string) ([]hookEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []hookEntry
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var h hookEntry
		if err := json.Unmarshal([]byte(line), &h); err != nil {
			return nil, fmt.Errorf("bad jsonl line: %w", err)
		}
		out = append(out, h)
	}
	return out, nil
}

// ---------- server 进程观测 ----------

// hwmKB 解析 /proc/<pid>/status 文本的 VmHWM（峰值 RSS，kB）
func hwmKB(status string) (int, error) {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			var kb int
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "VmHWM:"), "%d", &kb); err != nil {
				return 0, fmt.Errorf("parse VmHWM: %v", err)
			}
			return kb, nil
		}
	}
	return 0, fmt.Errorf("VmHWM not found")
}

func readHWM(serverPidFile string) (int, error) {
	b, err := os.ReadFile(serverPidFile)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pid := strings.TrimSpace(string(b))
	if pid == "" {
		return 0, fmt.Errorf("empty pid file")
	}
	st, err := os.ReadFile("/proc/" + pid + "/status")
	if err != nil {
		return 0, fmt.Errorf("read status: %w", err)
	}
	return hwmKB(string(st))
}

// ---------- 调度 ----------

// dailyAt 生成 now+offset 分钟落点的 daily@HH:mm 调度串（分钟粒度跨小时翻转安全）
func dailyAt(now time.Time, offsetMin int) string {
	t := now.Truncate(time.Minute).Add(time.Duration(offsetMin) * time.Minute)
	return fmt.Sprintf("daily@%02d:%02d", t.Hour(), t.Minute())
}

// dueOf daily@ 调度串的本轮到点时刻（与 NextBackupRunAt 同语义）
func dueOf(now time.Time, offsetMin int) time.Time {
	return now.Truncate(time.Minute).Add(time.Duration(offsetMin) * time.Minute)
}
