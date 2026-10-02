package server

import (
	"strings"
	"testing"
)

// SSH 密钥自动获取的门控：显式凭据（口令/密钥）必须原样透传，
// 空凭据才向 agent 拉默认密钥兜底。回归背景：首版只在 private_key
// 判空，口令认证被劫持成公钥认证（验收探针 S1/S5-S9 全挂）。
func TestApplySSHDefaultKeyGating(t *testing.T) {
	const agentPEM = "-----BEGIN OPENSSH PRIVATE KEY-----\nagent\n-----END OPENSSH PRIVATE KEY-----\n"

	orig := sshDefaultKeyLookup
	defer func() { sshDefaultKeyLookup = orig }()

	called := 0
	sshDefaultKeyLookup = func(_ *Server, _ string) (string, string, error) {
		called++
		return agentPEM, "agentuser", nil
	}

	s := &Server{}

	t.Run("显式口令→不拉密钥、参数原样", func(t *testing.T) {
		params := map[string]string{"username": "accept", "password": "pw"}
		s.applySSHDefaultKey(params, "agent-1", "127.0.0.1")
		if called != 0 {
			t.Fatalf("password 已给出时不应查询 agent 密钥，calls=%d", called)
		}
		if params["password"] != "pw" || params["username"] != "accept" {
			t.Fatalf("显式凭据被篡改: %+v", params)
		}
		if params["private_key"] != "" {
			t.Fatalf("不应注入 private_key: %q", params["private_key"])
		}
	})

	t.Run("显式私钥→不拉密钥、不覆盖 username", func(t *testing.T) {
		params := map[string]string{"username": "ops", "private_key": "K"}
		s.applySSHDefaultKey(params, "agent-1", "h")
		if called != 0 {
			t.Fatalf("private_key 已给出时不应查询 agent 密钥，calls=%d", called)
		}
		if params["private_key"] != "K" || params["username"] != "ops" {
			t.Fatalf("显式凭据被篡改: %+v", params)
		}
	})

	t.Run("空凭据→注入 agent 密钥并补 username", func(t *testing.T) {
		params := map[string]string{}
		s.applySSHDefaultKey(params, "agent-1", "h")
		if called != 1 {
			t.Fatalf("空凭据应恰好查询一次，calls=%d", called)
		}
		if params["private_key"] != agentPEM {
			t.Fatalf("agent 密钥未注入: %q", params["private_key"])
		}
		if params["username"] != "agentuser" {
			t.Fatalf("agent username 未补: %q", params["username"])
		}
	})

	t.Run("空凭据但已有 username→只注入密钥", func(t *testing.T) {
		params := map[string]string{"username": "accept"}
		s.applySSHDefaultKey(params, "agent-1", "h")
		if params["private_key"] == "" || params["username"] != "accept" {
			t.Fatalf("username 被覆盖或密钥未注入: %+v", params)
		}
	})

	t.Run("注入的必须是 PEM 原文形态（非 base64）", func(t *testing.T) {
		params := map[string]string{}
		s.applySSHDefaultKey(params, "agent-1", "h")
		if !strings.Contains(params["private_key"], "BEGIN OPENSSH PRIVATE KEY") {
			t.Fatalf("private-key 参数需 PEM 原文（guacd 零 base64 解码）: %q", params["private_key"])
		}
	})
}
