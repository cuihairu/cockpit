// REST 客户端、备份视图解析与配置/运行/恢复 API 封装（自 main.go 按职责拆分）。
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	token   string
	httpCli = &http.Client{Timeout: 10 * time.Minute}
)

// ---------- REST 层 ----------

func u(path string) string { return *apiBase + path }

func req(method, path string, body interface{}, tok string) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return -1, nil
		}
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, u(path), rd)
	if err != nil {
		return -1, nil
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpCli.Do(r)
	if err != nil {
		return -1, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func loginAs(user, pass string) (string, error) {
	code, raw := req(http.MethodPost, "/api/auth/login",
		map[string]string{"username": user, "password": pass}, "")
	if code != http.StatusOK {
		return "", fmt.Errorf("login HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Token == "" {
		return "", fmt.Errorf("login token parse: %v", err)
	}
	return out.Token, nil
}

// ---------- 解析层 ----------

type backupConfig struct {
	ID         uint     `json:"id"`
	AgentID    string   `json:"agent_id"`
	Name       string   `json:"name"`
	Sources    []string `json:"sources"`
	DestDir    string   `json:"dest_dir"`
	Schedule   string   `json:"schedule"`
	Retention  int      `json:"retention"`
	Enabled    bool     `json:"enabled"`
	NextRunAt  int64    `json:"next_run_at"`
	LastStatus string   `json:"last_status"`
}

func parseConfigView(raw []byte) (*backupConfig, error) {
	var c backupConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func parseConfigList(raw []byte) ([]backupConfig, error) {
	var out struct {
		Configs []backupConfig `json:"configs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Configs, nil
}

type backupRun struct {
	ID         uint   `json:"id"`
	Status     string `json:"status"`
	File       string `json:"file"`
	Size       int64  `json:"size"`
	Error      string `json:"error"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
}

func parseRuns(raw []byte) ([]backupRun, error) {
	var out struct {
		Runs []backupRun `json:"runs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Runs, nil
}

type backupFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func parseFiles(raw []byte) ([]backupFile, error) {
	var out struct {
		Files []backupFile `json:"files"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Files, nil
}

type taskView struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`
	Error  string `json:"error"`
	Log    string `json:"log"`
}

func parseTask(raw []byte) (taskView, error) {
	var t taskView
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, err
	}
	return t, nil
}

// ---------- 配置/运行/恢复 API 封装 ----------

func newConfig(name string, sources []string, dest, schedule string, retention int) (*backupConfig, error) {
	body := map[string]interface{}{
		"agent_id": *agentID, "name": name, "sources": sources,
		"dest_dir": dest, "schedule": schedule, "retention": retention,
	}
	code, raw := req(http.MethodPost, "/api/backups/configs", body, token)
	if code != http.StatusCreated {
		return nil, fmt.Errorf("create config HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return parseConfigView(raw)
}

func configByID(id uint) (*backupConfig, error) {
	code, raw := req(http.MethodGet, "/api/backups/configs", nil, token)
	if code != http.StatusOK {
		return nil, fmt.Errorf("configs HTTP %d: %s", code, truncate(string(raw), 200))
	}
	list, err := parseConfigList(raw)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("config %d not found", id)
}

func runsOf(cfgID uint) ([]backupRun, error) {
	code, raw := req(http.MethodGet, fmt.Sprintf("/api/backups/configs/%d/runs", cfgID), nil, token)
	if code != http.StatusOK {
		return nil, fmt.Errorf("runs HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return parseRuns(raw)
}

// waitTerminalRun 轮询该配置运行历史最新一条直到终态
func waitTerminalRun(cfgID uint, deadline time.Time) (backupRun, error) {
	var last string
	for time.Now().Before(deadline) {
		runs, err := runsOf(cfgID)
		if err != nil {
			last = err.Error()
		} else if len(runs) == 0 {
			last = "no runs yet"
		} else if runs[0].Status == "running" {
			last = "still running"
		} else {
			return runs[0], nil
		}
		time.Sleep(*pollStep)
	}
	return backupRun{}, fmt.Errorf("run not terminal before deadline (last: %s)", last)
}

func runConfig(cfgID uint) error {
	code, raw := req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/run", cfgID), nil, token)
	if code != http.StatusOK {
		return fmt.Errorf("run HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return nil
}

func filesOf(cfgID uint) ([]backupFile, error) {
	code, raw := req(http.MethodGet, fmt.Sprintf("/api/backups/configs/%d/files", cfgID), nil, token)
	if code != http.StatusOK {
		return nil, fmt.Errorf("files HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return parseFiles(raw)
}

// downloadBackup 拉取备份文件：w 非 nil 时流式写入（K6 大包边读边哈希不驻留）
func downloadBackup(cfgID uint, name string, w io.Writer) (int, int64, string, error) {
	r, err := http.NewRequest(http.MethodGet,
		u(fmt.Sprintf("/api/backups/configs/%d/files/download?name=%s", cfgID, url.QueryEscape(name))), nil)
	if err != nil {
		return -1, 0, "", err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpCli.Do(r)
	if err != nil {
		return -1, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, 0, "", fmt.Errorf("download HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	h := sha256.New()
	var dst io.Writer = h
	if w != nil {
		dst = io.MultiWriter(w, h)
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return resp.StatusCode, 0, "", fmt.Errorf("download copy: %w", err)
	}
	return resp.StatusCode, resp.ContentLength, hex.EncodeToString(h.Sum(nil)), nil
}

func startRestore(cfgID uint, file, destDir, confirm string) (string, error) {
	code, raw := req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/restore", cfgID),
		map[string]string{"file": file, "dest_dir": destDir, "confirm_name": confirm}, token)
	if code != http.StatusOK {
		return "", fmt.Errorf("restore HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var out struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.TaskID == "" {
		return "", fmt.Errorf("restore task id parse: %v", err)
	}
	return out.TaskID, nil
}

func waitTerminalTask(cfgID uint, taskID string, deadline time.Time) (taskView, error) {
	var last string
	for time.Now().Before(deadline) {
		code, raw := req(http.MethodGet,
			fmt.Sprintf("/api/backups/configs/%d/tasks/%s", cfgID, taskID), nil, token)
		if code != http.StatusOK {
			last = fmt.Sprintf("task HTTP %d", code)
		} else if t, err := parseTask(raw); err != nil {
			last = err.Error()
		} else if t.Status != "running" {
			return t, nil
		} else {
			last = "still running"
		}
		time.Sleep(*pollStep)
	}
	return taskView{}, fmt.Errorf("task not terminal before deadline (last: %s)", last)
}
