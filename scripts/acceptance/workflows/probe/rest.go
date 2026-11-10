// Workflow 验收 cockpit REST 客户端（自 main.go 按职责拆分）：请求发送、
// 登录取 token、agent 在线查询与多用户身份切换（reqJSON/loginAs/login/
// agentOnline/setToken + token/httpC 状态）。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	token string
	httpC = &http.Client{Timeout: 30 * time.Second}
)

// ---------- cockpit REST ----------

func reqJSON(method, url string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	evMu.Lock()
	tk := token
	evMu.Unlock()
	if tk != "" {
		req.Header.Set("Authorization", "Bearer "+tk)
	}
	resp, err := httpC.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// loginAs 登录取 token（不改包级 token——W8 多用户场景用）
func loginAs(user, pass string) (string, error) {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/auth/login",
		map[string]string{"username": user, "password": pass})
	if code != 200 {
		return "", fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Token == "" {
		return "", fmt.Errorf("响应无 token: %s", truncate(string(raw), 200))
	}
	return r.Token, nil
}

func login() {
	tk, err := loginAs(*adminUser, *adminPass)
	if err != nil {
		fatal("登录失败: %v", err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()
}

// agentOnline /api/agents 为 DB 视图（裸数组与 {agents:[..]} 包装两形态
// 兼容），按 id 找 status=online
func agentOnline(id string) (bool, error) {
	_, raw := reqJSON(http.MethodGet, *apiBase+"/api/agents", nil)
	type agentEntry struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	match := func(list []agentEntry) (bool, error) {
		for _, a := range list {
			if a.ID == id {
				return a.Status == "online", nil
			}
		}
		return false, fmt.Errorf("agent %s 不在 /api/agents 列表", id)
	}
	var list []agentEntry
	if json.Unmarshal(raw, &list) == nil {
		return match(list)
	}
	var wrapper struct {
		Agents []agentEntry `json:"agents"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return false, fmt.Errorf("bad /api/agents: %v", err)
	}
	return match(wrapper.Agents)
}

// setToken 切换请求身份（场景内多用户切换的统一口子）
func setToken(tk string) {
	evMu.Lock()
	token = tk
	evMu.Unlock()
}
