package server

// api_oob_redirect_test.go 兜底转发编排端点测试（片C）：fake agent 应答
// oob.redirect / oob.redirect.remove / oob.redirect.list，内存 fake vault
// 供 oob/<name> 条目解析；断言参数转发、双确认、错误映射与审计落库。
// 无真实 ubus 调用（规则读写在 agent 侧 provider 与 openwrt 客户端，已单测）。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/vault"
)

// oobFakeVault 内存 fake vault（只服务 oob/ 前缀，够本组测试用）。
// locked 置真时 Read 返回 ErrLocked，覆盖「vault 未解锁」的降级路径。
type oobFakeVault struct {
	entries map[string]*vault.Entry
	locked  bool
	err     error // Read 直接返回（覆盖非 locked/not-found 的通用错误分支）
}

func (f *oobFakeVault) Name() string        { return "fake" }
func (f *oobFakeVault) Unlock(string) error { return nil }
func (f *oobFakeVault) Lock()               {}
func (f *oobFakeVault) Read(path string) (*vault.Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.locked {
		return nil, vault.ErrLocked
	}
	e, ok := f.entries[path]
	if !ok {
		return nil, vault.ErrNotFound
	}
	return e, nil
}
func (f *oobFakeVault) Write(string, *vault.Entry) error { return vault.ErrUnsupported }
func (f *oobFakeVault) List(string) ([]vault.EntryMeta, error) {
	return nil, vault.ErrUnsupported
}
func (f *oobFakeVault) Delete(string) error { return vault.ErrUnsupported }

func gwEntry(extra map[string]string) *vault.Entry {
	meta := map[string]string{"addr": "10.0.0.9:8443"}
	for k, v := range extra {
		meta[k] = v
	}
	return &vault.Entry{Path: "oob/gw", Kind: "pair", Username: "root", Value: "pw", Meta: meta}
}

// rpcNum RPC 参数数值统一读法：进程内直传是 int，经 JSON 编解码是 float64。
func rpcNum(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return -1
}

func oobRedirectBody(extra string) string {
	return `{"agent_id":"a1","oob":"gw","spec":{"label":"amtbmc","proto":"tcp","src_zone":"wan",` +
		`"dst_zone":"lan","dst_ip":"192.168.1.10","dst_port":16992,"ttl_seconds":1800,` +
		`"src_ip":"10.0.0.5","src_port":51000},"confirm_label":"amtbmc"` + extra + `}`
}

func oobRedirectSuccessServer(t *testing.T) (*Server, map[string]interface{}) {
	t.Helper()
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{
		"oob/gw": gwEntry(map[string]string{"insecure_tls": "true"}),
	}}
	got := map[string]interface{}{}
	oobFakeAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		for k, v := range params {
			got[k] = v
		}
		switch method {
		case "oob.redirect":
			return map[string]interface{}{"name": "cockpit_amtbmc_1", "expiry": 1800, "ttl_seconds": 1800}, ""
		case "oob.redirect.remove":
			return map[string]interface{}{"name": "cockpit_amtbmc_1"}, ""
		}
		return nil, "unknown method " + method
	})
	return s, got
}

// ============ 下发 ============

func TestOobRedirectValidation(t *testing.T) {
	s, _ := oobRedirectSuccessServer(t)
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `nope`, http.StatusBadRequest},
		{"missing agent and oob", `{"spec":{}}`, http.StatusBadRequest},
		{"missing oob", `{"agent_id":"a1","spec":{}}`, http.StatusBadRequest},
		{"bad proto", strings.ReplaceAll(oobRedirectBody(`}`), `"proto":"tcp"`, `"proto":"icmp"`), http.StatusBadRequest},
		{"bad label", strings.ReplaceAll(oobRedirectBody(`}`), "amtbmc", "AMTBMC"), http.StatusBadRequest},
		{"bad dst ip", strings.ReplaceAll(oobRedirectBody(``), `"192.168.1.10"`, `"not-an-ip"`), http.StatusBadRequest},
		{"bad dst port", strings.ReplaceAll(oobRedirectBody(``), `"dst_port":16992`, `"dst_port":0`), http.StatusBadRequest},
		{"confirm mismatch", strings.ReplaceAll(oobRedirectBody(``), `"confirm_label":"amtbmc"`, `"confirm_label":"other"`), http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(c.body)))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestOobRedirectAgentOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
	covWantCode(t, "offline", rec, http.StatusServiceUnavailable)
}

func TestOobRedirectRequiresOobCapability(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	// 只带 backup capability 的 agent：oob 能力缺失必须 404
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Version: "1"}}, nil)
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
	covWantCode(t, "no oob cap", rec, http.StatusNotFound)
}

func TestOobRedirectVaultErrors(t *testing.T) {
	cases := []struct {
		name       string
		v          *oobFakeVault
		noProvider bool
		want       int
	}{
		{"no provider", nil, true, http.StatusServiceUnavailable},
		{"not found", &oobFakeVault{entries: map[string]*vault.Entry{}}, false, http.StatusServiceUnavailable},
		{"locked", &oobFakeVault{locked: true, entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}, false, http.StatusServiceUnavailable},
		{"wrong kind", &oobFakeVault{entries: map[string]*vault.Entry{
			"oob/gw": &vault.Entry{Path: "oob/gw", Kind: "api_token", Meta: map[string]string{"addr": "10.0.0.9"}}},
		}, false, http.StatusServiceUnavailable},
		{"no addr", &oobFakeVault{entries: map[string]*vault.Entry{
			"oob/gw": &vault.Entry{Path: "oob/gw", Kind: "pair", Meta: map[string]string{}}},
		}, false, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestServerWithDB(t)
			if c.noProvider {
				s.vaultProvider = nil
			} else {
				s.vaultProvider = c.v
			}
			oobFakeAgent(t, s, "a1", nil)
			rec := covRec()
			s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestOobRedirectForwardsRPC(t *testing.T) {
	s, got := oobRedirectSuccessServer(t)
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
	covWantCode(t, "apply", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"name":"cockpit_amtbmc_1"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	// 规格透传
	if got["label"] != "amtbmc" || got["proto"] != "tcp" || got["dst_ip"] != "192.168.1.10" ||
		rpcNum(got["dst_port"]) != 16992 || rpcNum(got["ttl_seconds"]) != 1800 ||
		got["src_ip"] != "10.0.0.5" || rpcNum(got["src_port"]) != 51000 {
		t.Fatalf("params = %v", got)
	}
	// 凭据从 vault 解析后下发（明文只在加密通道里）
	if got["host"] != "10.0.0.9" || rpcNum(got["port"]) != 8443 ||
		got["username"] != "root" || got["password"] != "pw" || got["insecure_tls"] != true {
		t.Fatalf("conn = %v", got)
	}

	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit = %+v", logs)
	}
}

func TestOobRedirectOptionalFieldsOmitted(t *testing.T) {
	s, got := oobRedirectSuccessServer(t)
	body := `{"agent_id":"a1","oob":"gw","spec":{"label":"b","proto":"udp","src_zone":"wan",` +
		`"dst_zone":"lan","dst_ip":"192.168.1.11","dst_port":16993},"confirm_label":"b"}`
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(body)))
	covWantCode(t, "apply minimal", rec, http.StatusOK)
	if _, has := got["src_ip"]; has {
		t.Fatalf("empty src_ip must be omitted: %v", got)
	}
	if _, has := got["src_port"]; has {
		t.Fatalf("zero src_port must be omitted: %v", got)
	}
}

func TestOobRedirectAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.redirect: failed to apply redirect: conn refused"
	})
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
	covWantCode(t, "agent error", rec, http.StatusBadGateway)

	// 拒绝也要留痕
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("refusal must be audited: %+v", logs)
	}
}

func TestOobRedirectCallAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleOobRedirect(rec, covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`))))
	covWantCode(t, "call error", rec, http.StatusBadGateway)
}

// ============ 拆除 ============

func TestOobRedirectRemoveValidation(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `nope`, http.StatusBadRequest},
		{"missing all", `{}`, http.StatusBadRequest},
		{"confirm mismatch", `{"agent_id":"a1","oob":"gw","name":"cockpit_a_1","confirm_name":"other"}`, http.StatusBadRequest},
		{"foreign rule", `{"agent_id":"a1","oob":"gw","name":"user_rule","confirm_name":"user_rule"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := covRec()
			s.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove", strings.NewReader(c.body)))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

func TestOobRedirectRemoveSuccess(t *testing.T) {
	s, got := oobRedirectSuccessServer(t)
	body := `{"agent_id":"a1","oob":"gw","name":"cockpit_amtbmc_1","confirm_name":"cockpit_amtbmc_1"}`
	rec := covRec()
	s.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove", strings.NewReader(body)))
	covWantCode(t, "remove", rec, http.StatusOK)
	if got["name"] != "cockpit_amtbmc_1" || got["host"] != "10.0.0.9" {
		t.Fatalf("params = %v", got)
	}
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirectRemove})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit = %+v", logs)
	}
}

func TestOobRedirectRemoveOffline(t *testing.T) {
	s := newTestServerWithDB(t)
	rec := covRec()
	s.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(`{"agent_id":"a1","oob":"gw","name":"cockpit_a_1","confirm_name":"cockpit_a_1"}`)))
	covWantCode(t, "offline", rec, http.StatusServiceUnavailable)
}

// ============ 读取 ============

func TestOobRedirectList(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}

	missing := covRec()
	s.handleOobRedirectList(missing, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1", nil))
	covWantCode(t, "missing oob", missing, http.StatusBadRequest)

	rec := covRec()
	s.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "list offline", rec, http.StatusServiceUnavailable)

	oobFakeAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "oob.redirect.list" || params["host"] != "10.0.0.9" {
			return nil, "unexpected call " + method
		}
		return map[string]interface{}{"redirects": []map[string]interface{}{{"name": "cockpit_amtbmc_1"}}}, ""
	})
	rec = covRec()
	s.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "list", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"redirects"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// ============ 路由 ============

func TestOobRedirectRouting(t *testing.T) {
	s := newTestServerWithDB(t)
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	authHeader := "Bearer " + token

	mux := http.NewServeMux()
	s.registerOobAPI(mux)

	apply := covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`)))
	apply.Header.Set("Authorization", authHeader)
	rec := covRec()
	mux.ServeHTTP(rec, apply)
	covWantCode(t, "apply route", rec, http.StatusServiceUnavailable) // agent offline

	list := covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil)
	list.Header.Set("Authorization", authHeader)
	rec = covRec()
	mux.ServeHTTP(rec, list)
	covWantCode(t, "list route", rec, http.StatusServiceUnavailable)

	remove := covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(`{"agent_id":"a1","oob":"gw","name":"cockpit_a_1","confirm_name":"cockpit_a_1"}`))
	remove.Header.Set("Authorization", authHeader)
	rec = covRec()
	mux.ServeHTTP(rec, remove)
	covWantCode(t, "remove route", rec, http.StatusServiceUnavailable)

	rec = covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/oob/redirect", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	del := covReq(http.MethodDelete, "/api/oob/redirect", nil)
	del.Header.Set("Authorization", authHeader)
	rec = covRec()
	mux.ServeHTTP(rec, del)
	covWantCode(t, "wrong method", rec, http.StatusNotFound)
}

// ============ vault 解析单元 ============

func TestVaultOobTarget(t *testing.T) {
	// host:port + insecure_tls
	v := &oobFakeVault{entries: map[string]*vault.Entry{
		"oob/gw": gwEntry(map[string]string{"insecure_tls": "true"})}}
	out, err := (&Server{vaultProvider: v}).vaultOobTarget("gw")
	if err != nil {
		t.Fatal(err)
	}
	if out["host"] != "10.0.0.9" || rpcNum(out["port"]) != 8443 ||
		out["username"] != "root" || out["password"] != "pw" || out["insecure_tls"] != true {
		t.Fatalf("out = %v", out)
	}

	// 仅主机名（无端口）：不带 port，也不带 insecure_tls
	v = &oobFakeVault{entries: map[string]*vault.Entry{
		"oob/gw": &vault.Entry{Path: "oob/gw", Kind: "pair", Meta: map[string]string{"addr": "10.0.0.9"}}}}
	out, err = (&Server{vaultProvider: v}).vaultOobTarget("gw")
	if err != nil {
		t.Fatal(err)
	}
	if out["host"] != "10.0.0.9" {
		t.Fatalf("out = %v", out)
	}
	if _, has := out["port"]; has {
		t.Fatalf("port must be omitted: %v", out)
	}
	if _, has := out["insecure_tls"]; has {
		t.Fatalf("insecure_tls must be omitted: %v", out)
	}

	// 空凭据不带键（匿名 ubus）
	v = &oobFakeVault{entries: map[string]*vault.Entry{
		"oob/gw": &vault.Entry{Path: "oob/gw", Kind: "password", Meta: map[string]string{"addr": "10.0.0.9"}}}}
	out, err = (&Server{vaultProvider: v}).vaultOobTarget("gw")
	if err != nil {
		t.Fatal(err)
	}
	if _, has := out["username"]; has {
		t.Fatalf("empty username must be omitted: %v", out)
	}
	if _, has := out["password"]; has {
		t.Fatalf("empty password must be omitted: %v", out)
	}
}

// TestTTLSeconds 秒转时长与缺省收敛。
func TestTTLSeconds(t *testing.T) {
	if ttlSeconds(0) != 0 || ttlSeconds(-5) != 0 {
		t.Fatalf("non-positive must be 0")
	}
	if got := ttlSeconds(1800); got != 1800*1000000000 {
		t.Fatalf("ttl = %v, want %v", got, 1800*1000000000)
	}
}

// ============ 拆除/读取 的错误分支 ============

func oobRedirectRemoveBody(name string) string {
	return `{"agent_id":"a1","oob":"gw","name":"` + name + `","confirm_name":"` + name + `"}`
}

func TestOobRedirectRemoveCapabilityAndVault(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Version: "1"}}, nil)
	rec := covRec()
	s.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(oobRedirectRemoveBody("cockpit_a_1"))))
	covWantCode(t, "no oob cap", rec, http.StatusNotFound)

	s2 := newTestServerWithDB(t)
	oobFakeAgent(t, s2, "a1", nil)
	s2.vaultProvider = &oobFakeVault{err: errors.New("backend down")}
	rec = covRec()
	s2.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(oobRedirectRemoveBody("cockpit_a_1"))))
	covWantCode(t, "vault error", rec, http.StatusServiceUnavailable)
}

func TestOobRedirectRemoveCallAgentError(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(oobRedirectRemoveBody("cockpit_a_1"))))
	covWantCode(t, "call error", rec, http.StatusBadGateway)

	// agent 侧拒绝：502 且留痕（用户名 unknown，直调无鉴权上下文）
	s2 := newTestServerWithDB(t)
	s2.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	oobFakeAgent(t, s2, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.redirect.remove: failed to remove redirect: no such rule"
	})
	rec = covRec()
	s2.handleOobRedirectRemove(rec, covReq(http.MethodPost, "/api/oob/redirect/remove",
		strings.NewReader(oobRedirectRemoveBody("cockpit_a_1"))))
	covWantCode(t, "agent refuse", rec, http.StatusBadGateway)
	logs, _, err := s2.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("refusal must be audited: %+v", logs)
	}
}

func TestOobRedirectListErrors(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	withFakeBackupAgentCap(t, s, "a1",
		[]protocol.Capability{{Type: "backup", Version: "1"}}, nil)
	rec := covRec()
	s.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "no oob cap", rec, http.StatusNotFound)

	s2 := newTestServerWithDB(t)
	oobFakeAgent(t, s2, "a1", nil)
	s2.vaultProvider = &oobFakeVault{err: errors.New("backend down")}
	rec = covRec()
	s2.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "vault error", rec, http.StatusServiceUnavailable)

	s3 := newTestServerWithDB(t)
	s3.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "oob"}}
	if err := s3.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s3.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "call error", rec, http.StatusBadGateway)

	s4 := newTestServerWithDB(t)
	s4.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	oobFakeAgent(t, s4, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.redirect.list: failed to list redirects: timeout"
	})
	rec = covRec()
	s4.handleOobRedirectList(rec, covReq(http.MethodGet, "/api/oob/redirect?agent_id=a1&oob=gw", nil))
	covWantCode(t, "agent refuse", rec, http.StatusBadGateway)
}

// TestOobRedirectAuditViaMux 走 mux + Bearer：审计 username 取登录用户，
// 覆盖三个审计函数的鉴权上下文分支。
func TestOobRedirectAuditViaMux(t *testing.T) {
	s, _ := oobRedirectSuccessServer(t)
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.registerOobAPI(mux)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := covReq(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := covRec()
		mux.ServeHTTP(rec, req)
		return rec
	}

	covWantCode(t, "apply", do(http.MethodPost, "/api/oob/redirect", oobRedirectBody(`}`)), http.StatusOK)
	covWantCode(t, "remove", do(http.MethodPost, "/api/oob/redirect/remove", oobRedirectRemoveBody("cockpit_amtbmc_1")), http.StatusOK)
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "admin" {
		t.Fatalf("apply audit = %+v", logs)
	}
	rlogs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirectRemove})
	if err != nil {
		t.Fatal(err)
	}
	if len(rlogs) != 1 || rlogs[0].Username != "admin" {
		t.Fatalf("remove audit = %+v", rlogs)
	}
}

// TestOobRedirectRefuseViaMux 鉴权上下文下的拒绝留痕（审计 username 为登录用户）。
func TestOobRedirectRefuseViaMux(t *testing.T) {
	s := newTestServerWithDB(t)
	s.vaultProvider = &oobFakeVault{entries: map[string]*vault.Entry{"oob/gw": gwEntry(nil)}}
	oobFakeAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return nil, "oob.redirect: failed to apply redirect: conn refused"
	})
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.registerOobAPI(mux)
	req := covReq(http.MethodPost, "/api/oob/redirect", strings.NewReader(oobRedirectBody(`}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "refuse", rec, http.StatusBadGateway)
	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobRedirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "admin" {
		t.Fatalf("refuse audit = %+v", logs)
	}
}

// TestVaultOobTargetGenericError 通用 vault 错误原样透传。
func TestVaultOobTargetGenericError(t *testing.T) {
	s := &Server{vaultProvider: &oobFakeVault{err: errors.New("backend down")}}
	if _, err := s.vaultOobTarget("gw"); err == nil || !strings.Contains(err.Error(), "backend down") {
		t.Fatalf("err = %v", err)
	}
}
