// REST 客户端（自 main.go 按职责拆分）：登录、远控票据、通用请求。
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

var token string

func api(method, path string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *apiBase+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func login() {
	for i := 0; i < 20; i++ {
		st, body := api("POST", "/api/auth/login", map[string]string{"username": *adminUser, "password": *adminPass})
		if st == 200 {
			var r struct {
				Token string `json:"token"`
			}
			if json.Unmarshal(body, &r) == nil && r.Token != "" {
				token = r.Token
				return
			}
		}
		time.Sleep(loginRetryWait)
	}
	fatal("登录失败")
}

// createTicket POST /api/remote/tickets，返回票据与 HTTP 状态码
func createTicket(p map[string]interface{}) (string, int, string) {
	st, body := api("POST", "/api/remote/tickets", p)
	if st != 200 {
		return "", st, string(body)
	}
	var r struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Ticket == "" {
		return "", st, "ticket 缺失: " + string(body)
	}
	return r.Ticket, st, ""
}
