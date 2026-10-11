package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/vault"
)

const vaultNasTestKey = "vault-nas-test-key-0123456789abcdef!"

// newVaultNasServer 带 local 密码箱 provider 的测试 Server（vault 键固定，
// 与既有 vault 测试键区分；经同一 in-memory DB 挂 vault_entries 表）。
func newVaultNasServer(t *testing.T) (*Server, vault.Provider) {
	t.Helper()
	vault.ApplyKey(vaultNasTestKey)
	s := newTestServerWithDB(t)
	p, err := initVault(&config.VaultConfig{}, s.db, s.audit)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	s.vaultProvider = p
	return s, p
}

func writeNasEntry(t *testing.T, p vault.Provider, name, username, password, typ, addr string) {
	t.Helper()
	meta := map[string]string{"type": typ, "addr": addr}
	if err := p.Write(nasVaultPrefix+name, &vault.Entry{
		Path: nasVaultPrefix + name, Kind: "pair", Username: username, Value: password, Meta: meta,
	}); err != nil {
		t.Fatalf("write nas entry %s: %v", name, err)
	}
}

func TestVaultNasTargetsMapping(t *testing.T) {
	s, p := newVaultNasServer(t)
	writeNasEntry(t, p, "box1", "admin", "pw-one", "dsm", "https://192.168.1.10:5001")
	writeNasEntry(t, p, "box2", "root", "pw-two", "omv", "https://192.168.1.11")
	// insecure_tls 标志
	if err := p.Write("nas/box3", &vault.Entry{
		Path: "nas/box3", Kind: "pair", Username: "u", Value: "pw-three",
		Meta: map[string]string{"type": "dsm", "addr": "https://192.168.1.12:5001", "insecure_tls": "true"},
	}); err != nil {
		t.Fatalf("write box3: %v", err)
	}
	// 非 nas 前缀条目不参与
	if err := p.Write("remote/other", &vault.Entry{Path: "remote/other", Kind: "pair", Value: "x"}); err != nil {
		t.Fatalf("write remote: %v", err)
	}

	targets, hasVault := s.vaultNasTargets()
	if !hasVault {
		t.Fatal("vault provider present, want hasVault")
	}
	if len(targets) != 3 {
		t.Fatalf("targets = %d, want 3", len(targets))
	}
	first := targets[0]
	if first["name"] != "box1" || first["type"] != "dsm" || first["username"] != "admin" || first["password"] != "pw-one" {
		t.Fatalf("mapping wrong: %v", first)
	}
	var insecureSeen bool
	for _, tmap := range targets {
		if tmap["name"] == "box3" {
			insecureSeen = tmap["insecureTls"] == true
		}
	}
	if !insecureSeen {
		t.Fatal("insecure_tls meta must map to insecureTls")
	}
}

func TestVaultNasTargetsCorruptEntrySkipped(t *testing.T) {
	s, p := newVaultNasServer(t)
	db := s.db
	writeNasEntry(t, p, "good", "u", "pw-good", "dsm", "https://h:5001")
	if err := p.Write("nas/bad", &vault.Entry{
		Path: "nas/bad", Kind: "pair", Username: "u", Value: "pw-bad",
		Meta: map[string]string{"type": "dsm", "addr": "https://h2"},
	}); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	// 损坏 bad 条目密文（v1 前缀去掉使解密失败）
	if err := db.Gorm().Table("vault_entries").Where("path = ?", "nas/bad").
		Update("value_enc", "corrupted").Error; err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	targets, _ := s.vaultNasTargets()
	if len(targets) != 1 || targets[0]["name"] != "good" {
		t.Fatalf("corrupt entry must be skipped: %v", targets)
	}
}

func TestPushNasTargets(t *testing.T) {
	s, p := newVaultNasServer(t)
	rpcCalls := make(chan string, 4)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "nas.set_targets" {
			// CallAgent 直传 native map（无 JSON 往返），形状是 []map[string]interface{}
			if ts, ok := params["targets"].([]map[string]interface{}); ok && len(ts) > 0 {
				rpcCalls <- ts[0]["password"].(string)
			}
		}
		return map[string]interface{}{"ok": true}, ""
	})
	writeNasEntry(t, p, "box1", "admin", "push-pw-XYZ", "dsm", "https://h:5001")

	if !s.pushNasTargets("a1") {
		t.Fatal("pushNasTargets should succeed")
	}
	select {
	case got := <-rpcCalls:
		if got != "push-pw-XYZ" {
			t.Fatalf("pushed password = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no RPC within timeout")
	}

	// vault 空 → 不推送
	if err := p.Delete("nas/box1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if s.pushNasTargets("a1") {
		t.Fatal("empty vault must not push")
	}
	// 离线 agent → false
	writeNasEntry(t, p, "box2", "u", "pw", "dsm", "https://h2")
	if s.pushNasTargets("missing-agent") {
		t.Fatal("offline agent must fail push")
	}
}

func TestRegisterRespPayloadNasTargets(t *testing.T) {
	s, p := newVaultNasServer(t)
	agent := NewAgent("a1", nil)

	// vault 空：响应无 nasTargets，不打标
	payload := s.registerRespPayload(agent)
	if _, ok := payload["nasTargets"]; ok {
		t.Fatal("empty vault must not carry nasTargets")
	}
	if agent.nasVaultTargets {
		t.Fatal("flag must stay false")
	}

	writeNasEntry(t, p, "box1", "admin", "boot-pw", "dsm", "https://h:5001")
	payload = s.registerRespPayload(agent)
	ts, ok := payload["nasTargets"].([]map[string]interface{})
	if !ok || len(ts) != 1 || ts[0]["password"] != "boot-pw" {
		t.Fatalf("nasTargets wrong: %v", payload["nasTargets"])
	}
	if !agent.nasVaultTargets {
		t.Fatal("agent must be flagged for scan inclusion")
	}
}

func TestNasScanAgents(t *testing.T) {
	s := &Server{registry: NewRegistry()}
	capAgent := NewAgent("cap", nil)
	capAgent.Capabilities = []protocol.Capability{{Type: "nas"}}
	vaultAgent := NewAgent("vault-flag", nil)
	vaultAgent.nasVaultTargets = true
	other := NewAgent("other", nil)
	other.Capabilities = []protocol.Capability{{Type: "docker"}}
	for _, a := range []*Agent{capAgent, vaultAgent, other} {
		if err := s.registry.Register(a); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	got := s.nasScanAgents()
	if len(got) != 2 {
		t.Fatalf("scan agents = %d, want 2", len(got))
	}
	for _, a := range got {
		if a.ID == "other" {
			t.Fatal("non-nas agent must be excluded")
		}
	}
}

func TestVaultEntriesAPI(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	vault.ApplyKey(vaultNasTestKey)
	p, err := initVault(&config.VaultConfig{}, s.db, nil)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	s.vaultProvider = p
	token := vaultVerifyLogin(t, s)

	// 列表（空）
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/entries?prefix=nas/", nil))
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntries)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}

	// 写入缺 vault token → 401
	putBody := `{"kind":"pair","username":"admin","value":"api-pw-XYZ","meta":{"type":"dsm","addr":"https://h:5001"}}`
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/api1", strings.NewReader(putBody)))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("put without token = %d, want 401", rec.Code)
	}

	// 非法 path → 400
	for _, bad := range []string{"/api/remote/vault/entries/", "/api/remote/vault/entries/nas/../etc", "/api/remote/vault/entries/NAS/UP"} {
		req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, bad, strings.NewReader(putBody)))
		rec = httptest.NewRecorder()
		s.authService().Middleware(s.handleVaultEntry)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path %q status = %d, want 400", bad, rec.Code)
		}
	}

	// 写入（带 token）→ 200，落库无明文
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/api1", strings.NewReader(putBody)))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, err := p.Read("nas/api1")
	if err != nil || got.Value != "api-pw-XYZ" || got.Username != "admin" || got.Meta["type"] != "dsm" {
		t.Fatalf("read back: %v %+v", err, got)
	}

	// 列表回元数据不含明文
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodGet, "/api/remote/vault/entries", nil))
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntries)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "api-pw-XYZ") {
		t.Fatal("list must not leak plaintext")
	}

	// 删除（带 token）→ 200；再删 → 404
	delReq := func() int {
		req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/entries/nas/api1", nil))
		req.Header.Set("X-Vault-Token", token)
		rec := httptest.NewRecorder()
		s.authService().Middleware(s.handleVaultEntry)(rec, req)
		return rec.Code
	}
	if code := delReq(); code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
	if code := delReq(); code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", code)
	}
}

func TestVaultEntriesAPINasChangeTriggersPush(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	vault.ApplyKey(vaultNasTestKey)
	p, err := initVault(&config.VaultConfig{}, s.db, nil)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	s.vaultProvider = p
	token := vaultVerifyLogin(t, s)

	pushed := make(chan string, 2)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "nas.set_targets" {
			pushed <- method
		}
		return map[string]interface{}{"ok": true}, ""
	})

	body := `{"username":"u","value":"v"}`
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/live", strings.NewReader(body)))
	req.Header.Set("X-Vault-Token", token)
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d", rec.Code)
	}
	select {
	case m := <-pushed:
		if m != "nas.set_targets" {
			t.Fatalf("method = %q", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nas entry change must trigger push")
	}

	// 非 nas 前缀不触发
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/remote/x", strings.NewReader(body)))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put remote status = %d", rec.Code)
	}
	select {
	case <-pushed:
		t.Fatal("non-nas entry must not push")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestVaultEntriesAPIAuditTrail(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	vault.ApplyKey(vaultNasTestKey)
	p, err := initVault(&config.VaultConfig{}, s.db, nil)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	s.vaultProvider = p
	token := vaultVerifyLogin(t, s)

	const secret = "audit-entry-9q"
	body := `{"username":"entry-user-9q","value":"` + secret + `"}`
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/aud", strings.NewReader(body)))
	req.Header.Set("X-Vault-Token", token)
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d", rec.Code)
	}
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/entries/nas/aud", nil))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}

	var actions []string
	if err := s.db.Gorm().Table("audit_logs").Where("action IN ?", []string{"update", "delete"}).
		Order("id").Select("action").Scan(&actions).Error; err != nil {
		t.Fatalf("audit query: %v", err)
	}
	foundUpdate, foundDelete := false, false
	for _, a := range actions {
		if a == "update" {
			foundUpdate = true
		}
		if a == "delete" {
			foundDelete = true
		}
	}
	if !foundUpdate || !foundDelete {
		t.Fatalf("vault entry audit rows missing: %v", actions)
	}
	var details []string
	s.db.Gorm().Table("audit_logs").Where("resource_id = ?", "nas/aud").Select("details").Scan(&details)
	for _, d := range details {
		if strings.Contains(d, secret) || strings.Contains(d, "entry-user-9q") {
			t.Fatal("audit details must not contain plaintext")
		}
	}
}

// TestVaultEntriesAPIErrorBranches 全部确定性错误分支：405/401/503/400/500。
func TestVaultEntriesAPIErrorBranches(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	vault.ApplyKey(vaultNasTestKey)
	p, err := initVault(&config.VaultConfig{}, s.db, s.audit)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	s.vaultProvider = p

	// 405：entry 只收 PUT/DELETE，collection 只收 GET
	req := httptest.NewRequest(http.MethodGet, "/api/remote/vault/entries/nas/x", nil)
	rec := httptest.NewRecorder()
	s.handleVaultEntry(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET entry status = %d, want 405", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/remote/vault/entries", nil)
	rec = httptest.NewRecorder()
	s.handleVaultEntries(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST collection status = %d, want 405", rec.Code)
	}

	// 401：无认证上下文（方法合法但中间件未注入用户）
	req = httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/x", nil)
	rec = httptest.NewRecorder()
	s.handleVaultEntry(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no user entry status = %d, want 401", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/remote/vault/entries", nil)
	rec = httptest.NewRecorder()
	s.handleVaultEntries(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no user collection status = %d, want 401", rec.Code)
	}

	// 503：vault provider 未配置（认证后）
	token := vaultVerifyLogin(t, s)
	authedAdmin := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/x", strings.NewReader(`{"value":"v"}`)))
	adminAuthHeader := authedAdmin.Header.Get("Authorization")
	mkAuthed := func(method, path, body string) *http.Request {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		r := httptest.NewRequest(method, path, rd)
		r.Header.Set("Authorization", adminAuthHeader)
		r.Header.Set("X-Vault-Token", token)
		return r
	}
	s.vaultProvider = nil
	req = mkAuthed(http.MethodPut, "/api/remote/vault/entries/nas/x", `{"value":"v"}`)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil provider put status = %d, want 503", rec.Code)
	}
	req = mkAuthed(http.MethodGet, "/api/remote/vault/entries", "")
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntries)(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil provider list status = %d, want 503", rec.Code)
	}
	s.vaultProvider = p

	// 400：非法 JSON 体
	req = mkAuthed(http.MethodPut, "/api/remote/vault/entries/nas/x", "{bad json")
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json status = %d, want 400", rec.Code)
	}

	// 400：value 与 username 全空
	req = mkAuthed(http.MethodPut, "/api/remote/vault/entries/nas/x", `{"kind":"pair"}`)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty fields status = %d, want 400", rec.Code)
	}

	// 500：库已关 → 写/删/列全部失败（写删走失败审计分支）
	if err := s.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	req = mkAuthed(http.MethodPut, "/api/remote/vault/entries/nas/x", `{"value":"v"}`)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed-db put status = %d, want 500", rec.Code)
	}
	req = mkAuthed(http.MethodDelete, "/api/remote/vault/entries/nas/x", "")
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed-db delete status = %d, want 500", rec.Code)
	}
	req = mkAuthed(http.MethodGet, "/api/remote/vault/entries", "")
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntries)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed-db list status = %d, want 500", rec.Code)
	}
}

// vault List 失败降级：provider 在（hasVault=true）但读不出条目（nil）。
func TestVaultNasTargetsListFailure(t *testing.T) {
	s, _ := newVaultNasServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	targets, hasVault := s.vaultNasTargets()
	if !hasVault || targets != nil {
		t.Fatalf("list failure must return (nil, true), got (%v, %v)", targets, hasVault)
	}
}
