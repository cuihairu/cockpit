package server

// cov_ssh_default_lookup_test.go 覆盖 sshDefaultKeyLookup 默认闭包体
// （生产路径打真实 agent RPC，此前测试只测注入桩）与 applySSHDefaultKey
// 的查询失败分支。假 agent 经 covFakeAgent 走真实 RPC 往返。

import "testing"

func TestSSHDefaultKeyLookupClosure(t *testing.T) {
	s := covNewServer(t)

	// 1) agent 不在册 → CallAgent 报错
	if _, _, err := sshDefaultKeyLookup(s, "agent-none"); err == nil {
		t.Error("sshDefaultKeyLookup with unregistered agent should fail")
	}

	// 2) agent 应答 data map → 私钥/用户名解出
	covFakeAgent(t, s, "agent-key", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if method != "ssh.getDefaultKey" {
			t.Errorf("RPC method = %q, want ssh.getDefaultKey", method)
		}
		return covOKPayload(map[string]interface{}{"privateKey": "PEM-BODY", "username": "agentuser"})
	})
	pem, user, err := sshDefaultKeyLookup(s, "agent-key")
	if err != nil {
		t.Fatalf("sshDefaultKeyLookup: %v", err)
	}
	if pem != "PEM-BODY" || user != "agentuser" {
		t.Errorf("lookup = (%q, %q), want (PEM-BODY, agentuser)", pem, user)
	}

	// 3) 应答无 data map → 空 PEM/用户名、无错误
	covFakeAgent(t, s, "agent-key-nodata", nil, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "success"}
	})
	pem, user, err = sshDefaultKeyLookup(s, "agent-key-nodata")
	if err != nil {
		t.Fatalf("sshDefaultKeyLookup(no data): %v", err)
	}
	if pem != "" || user != "" {
		t.Errorf("lookup(no data) = (%q, %q), want empty", pem, user)
	}
}

// TestApplySSHDefaultKeyLookupError 覆盖 applySSHDefaultKey 的查询失败分支
// （默认闭包 + 未在册 agent）：仅记日志，参数不动
func TestApplySSHDefaultKeyLookupError(t *testing.T) {
	s := covNewServer(t)
	params := map[string]string{}
	s.applySSHDefaultKey(params, "agent-none", "h")
	if params["private_key"] != "" || params["username"] != "" {
		t.Errorf("params mutated on lookup error: %+v", params)
	}
}
