package vault

// persona provider 测试：假 Connect 服务端覆盖握手/失败路径/条目映射/
// 会话门/TTL/审计/注册表端到端。共享状态全部 mutex/atomic（-race 腿口径）。

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakePersona 假 persona Connect 服务端（/api/v1/connect 三端点 + 包络 +
// 401/503/404 语义）。
type fakePersona struct {
	mu       sync.Mutex
	items    map[string]any // id → 详情（含 data 变体；可放 json.RawMessage 投毒）
	token    string
	locked   bool
	noHealth bool // health 返回 500
	poison   bool // items 列表返回非数组（解码错误分支）
	srv      *httptest.Server
	reqs     atomic.Int32
}

func newFakePersona(t *testing.T) *fakePersona {
	t.Helper()
	f := &fakePersona{items: map[string]any{}, token: "pconn_test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/connect/health", func(w http.ResponseWriter, r *http.Request) {
		f.reqs.Add(1)
		f.mu.Lock()
		bad := f.noHealth
		f.mu.Unlock()
		if bad {
		personaErr(w, http.StatusInternalServerError, "internal", "boom")
			return
		}
		personaOK(w, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/connect/items", func(w http.ResponseWriter, r *http.Request) {
		f.reqs.Add(1)
		if !f.auth(r) {
			personaErr(w, http.StatusUnauthorized, "unauthorized", "bad token")
			return
		}
		f.mu.Lock()
		locked := f.locked
		poison := f.poison
		items := make([]map[string]any, 0, len(f.items))
		for id, raw := range f.items {
			m, ok := raw.(map[string]any)
			if !ok {
				continue // 投毒条目只经详情端点出现
			}
			items = append(items, map[string]any{
				"id": id, "title": m["title"], "type": m["type"],
				"urls": m["urls"], "updated_at": m["updated_at"],
			})
		}
		f.mu.Unlock()
		if locked {
			personaErr(w, http.StatusServiceUnavailable, "vault_locked", "owner must unlock")
			return
		}
		if poison {
			personaOK(w, 7) // 非数组：解码错误分支
			return
		}
		personaOK(w, items)
	})
	mux.HandleFunc("/api/v1/connect/items/", func(w http.ResponseWriter, r *http.Request) {
		f.reqs.Add(1)
		if !f.auth(r) {
			personaErr(w, http.StatusUnauthorized, "unauthorized", "bad token")
			return
		}
		f.mu.Lock()
		locked := f.locked
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/connect/items/")
		item := f.items[id]
		f.mu.Unlock()
		if locked {
			personaErr(w, http.StatusServiceUnavailable, "vault_locked", "owner must unlock")
			return
		}
		if item == nil {
			personaErr(w, http.StatusNotFound, "not_found", "no such item")
			return
		}
		personaOK(w, item)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePersona) auth(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+f.token
}

func (f *fakePersona) add(id, title, typ string, data map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[id] = map[string]any{
		"id": id, "title": title, "type": typ, "username": "root",
		"urls": []string{"https://example.example"}, "updated_at": "2026-10-13T00:00:00Z",
		"data": data,
	}
}

// addRaw 注入任意详情载荷（解码错误分支用）。
func (f *fakePersona) addRaw(id string, raw any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[id] = raw
}

func (f *fakePersona) setLocked(v bool) { f.mu.Lock(); f.locked = v; f.mu.Unlock() }

func personaOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

func personaErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false, "error": map[string]any{"code": code, "message": msg},
	})
}

func personaCfg(f *fakePersona) map[string]string {
	return map[string]string{"endpoint": f.srv.URL, "token": f.token}
}

func TestPersonaNewValidation(t *testing.T) {
	if _, err := NewPersona(map[string]string{"token": "pconn_x"}, 0, nil); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("want endpoint required, got %v", err)
	}
	if _, err := NewPersona(map[string]string{"endpoint": "127.0.0.1:1", "token": "t"}, 0, nil); err == nil || !strings.Contains(err.Error(), "http") {
		t.Fatalf("want scheme error, got %v", err)
	}
	if _, err := NewPersona(map[string]string{"endpoint": "http://127.0.0.1:1"}, 0, nil); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("want token required, got %v", err)
	}
	if _, err := NewPersona(map[string]string{"endpoint": "http://x", "token": "t", "timeout": "soon"}, 0, nil); err == nil {
		t.Fatal("want bad timeout error")
	}
	if _, err := NewPersona(map[string]string{"endpoint": "http://x", "token": "t", "timeout": "-1s"}, 0, nil); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("want positive timeout error, got %v", err)
	}
	p, err := NewPersona(map[string]string{"endpoint": "http://127.0.0.1:1", "token": "t"}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.client.Timeout != personaDefaultTimeout {
		t.Fatalf("default timeout: got %v", p.client.Timeout)
	}
	if p.Name() != "persona" {
		t.Fatalf("name: got %q", p.Name())
	}
}

func TestPersonaUnlockHandshake(t *testing.T) {
	f := newFakePersona(t)
	p, err := NewPersona(personaCfg(f), time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Unlock("anything"); err != nil { // passphrase 无语义，空串也合法
		t.Fatalf("unlock: %v", err)
	}
	if err := p.Unlock(""); err != nil {
		t.Fatalf("re-unlock idempotent: %v", err)
	}
	if err := p.sess.Check(); err != nil {
		t.Fatalf("session should be open: %v", err)
	}
}

func TestPersonaUnlockFailClosed(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		dead.Close() // 端口已关
		p, err := NewPersona(map[string]string{"endpoint": dead.URL, "token": "t", "timeout": "2s"}, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = p.Unlock("")
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("want ErrLocked, got %v", err)
		}
	})
	t.Run("vault locked 503", func(t *testing.T) {
		f := newFakePersona(t)
		f.setLocked(true)
		p, _ := NewPersona(personaCfg(f), 0, nil)
		err := p.Unlock("")
		if !errors.Is(err, ErrLocked) || !strings.Contains(err.Error(), "vault_locked") {
			t.Fatalf("want ErrLocked(vault_locked), got %v", err)
		}
	})
	t.Run("health down", func(t *testing.T) {
		f := newFakePersona(t)
		f.mu.Lock()
		f.noHealth = true
		f.mu.Unlock()
		p, _ := NewPersona(personaCfg(f), 0, nil)
		if err := p.Unlock(""); !errors.Is(err, ErrLocked) {
			t.Fatalf("want ErrLocked, got %v", err)
		}
	})
	t.Run("token rejected is not locked", func(t *testing.T) {
		f := newFakePersona(t)
		cfg := personaCfg(f)
		cfg["token"] = "pconn_wrong"
		p, _ := NewPersona(cfg, 0, nil)
		err := p.Unlock("")
		if err == nil || errors.Is(err, ErrLocked) {
			t.Fatalf("want explicit token error (not ErrLocked), got %v", err)
		}
		if !strings.Contains(err.Error(), "unauthorized") {
			t.Fatalf("want unauthorized in message, got %v", err)
		}
		if strings.Contains(err.Error(), "pconn_wrong") {
			t.Fatal("token must never appear in error message")
		}
	})
	t.Run("bad envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html>not json</html>"))
		}))
		defer srv.Close()
		p, _ := NewPersona(map[string]string{"endpoint": srv.URL, "token": "t"}, 0, nil)
		if err := p.Unlock(""); !errors.Is(err, ErrLocked) {
			t.Fatalf("want ErrLocked, got %v", err)
		}
	})
}

func TestPersonaSessionGateNoRequest(t *testing.T) {
	f := newFakePersona(t)
	f.add("11111111-1111-1111-1111-111111111111", "web", "password",
		map[string]any{"Password": map[string]any{"password": "s3cret"}})
	p, _ := NewPersona(personaCfg(f), 0, nil)
	id := "11111111-1111-1111-1111-111111111111"
	if _, err := p.Read(id); !errors.Is(err, ErrLocked) {
		t.Fatalf("read locked: %v", err)
	}
	if _, err := p.List(""); !errors.Is(err, ErrLocked) {
		t.Fatalf("list locked: %v", err)
	}
	if err := p.Write(id, &Entry{Value: "x"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("write locked: %v", err)
	}
	if err := p.Delete(id); !errors.Is(err, ErrLocked) {
		t.Fatalf("delete locked: %v", err)
	}
	if n := f.reqs.Load(); n != 0 {
		t.Fatalf("locked ops must not hit the wire, got %d requests", n)
	}
	p.Lock() // 未解锁时 Lock 也安全
	if _, err := p.Read(id); !errors.Is(err, ErrLocked) {
		t.Fatalf("read locked: %v", err)
	}
	// 参数校验先于会话门（与 local 一致）
	if _, err := p.Read(""); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("empty path: %v", err)
	}
	if err := p.Write(id, nil); err == nil || !strings.Contains(err.Error(), "nil entry") {
		t.Fatalf("nil entry: %v", err)
	}
}

func TestPersonaReadMappings(t *testing.T) {
	f := newFakePersona(t)
	cases := []struct {
		id, typ string
		data    map[string]any
		value   string
	}{
		{"id-password", "password", map[string]any{"Password": map[string]any{"password": "pw1"}}, "pw1"},
		{"id-apikey", "api_key", map[string]any{"ApiKey": map[string]any{"api_key": "ak1"}}, "ak1"},
		{"id-totp", "totp", map[string]any{"TwoFactor": map[string]any{"secret_key": "JBSW"}}, "JBSW"},
		{"id-note", "note", map[string]any{"SecureNote": map[string]any{"note": "n1"}}, "n1"},
		{"id-game", "game_account", map[string]any{"GameToken": map[string]any{"secret_key": "g1"}}, "g1"},
		{"id-lic", "software_license", map[string]any{"SoftwareLicense": map[string]any{"license_key": "l1"}}, "l1"},
		// server_config.password 为 Option：null → 空值不报错
		{"id-srv", "server_config", map[string]any{"ServerConfig": map[string]any{"host": "h", "password": nil}}, ""},
		// 无单一秘密字段类型：值空 + persona_data 字段名清单
		{"id-card", "bank_card", map[string]any{"BankCard": map[string]any{"card_number": "1", "cvv": "2"}}, ""},
	}
	for _, c := range cases {
		f.add(c.id, "item "+c.id, c.typ, c.data)
	}
	p, _ := NewPersona(personaCfg(f), time.Hour, nil)
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		e, err := p.Read(c.id)
		if err != nil {
			t.Fatalf("read %s: %v", c.id, err)
		}
		if e.Value != c.value {
			t.Fatalf("read %s value: got %q want %q", c.id, e.Value, c.value)
		}
		if e.Kind != c.typ || e.Username != "root" || e.Path != c.id {
			t.Fatalf("read %s: kind=%s user=%q path=%s", c.id, e.Kind, e.Username, e.Path)
		}
		if e.Meta["persona_title"] != "item "+c.id || e.Meta["persona_updated_at"] == "" {
			t.Fatalf("read %s meta: %v", c.id, e.Meta)
		}
	}
	card, err := p.Read("id-card")
	if err != nil {
		t.Fatal(err)
	}
	if card.Meta["persona_data"] != "card_number,cvv" {
		t.Fatalf("bank_card persona_data: %q", card.Meta["persona_data"])
	}
	srv, _ := p.Read("id-srv")
	if _, ok := srv.Meta["persona_data"]; ok {
		t.Fatalf("server_config should not list data fields: %v", srv.Meta)
	}
	// 404 → ErrNotFound
	if _, err := p.Read("id-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// 变体与 type 不符 → 显式错误
	f.mu.Lock()
	f.items["id-bad"] = map[string]any{
		"id": "id-bad", "title": "x", "type": "password", "updated_at": "2026-10-13T00:00:00Z",
		"data": map[string]any{"SecureNote": map[string]any{"note": "x"}},
	}
	f.mu.Unlock()
	if _, err := p.Read("id-bad"); err == nil || !strings.Contains(err.Error(), "variant") {
		t.Fatalf("want variant mismatch, got %v", err)
	}
}

func TestPersonaListMeta(t *testing.T) {
	f := newFakePersona(t)
	f.add("aa-1", "one", "password", map[string]any{"Password": map[string]any{"password": "v"}})
	f.add("aa-2", "two", "bank_card", map[string]any{"BankCard": map[string]any{"card_number": "4"}})
	f.add("bb-1", "three", "note", map[string]any{"SecureNote": map[string]any{"note": "n"}})
	p, _ := NewPersona(personaCfg(f), 0, nil)
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	metas, err := p.List("aa-")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("prefix filter: got %d metas", len(metas))
	}
	byPath := map[string]EntryMeta{}
	for _, m := range metas {
		byPath[m.Path] = m
		if m.HasUsername {
			t.Fatalf("%s: list meta must not claim username", m.Path)
		}
	}
	if byPath["aa-1"].Kind != "password" {
		t.Fatalf("kind must be type slug, got %q", byPath["aa-1"].Kind)
	}
	if !byPath["aa-1"].HasValue || byPath["aa-2"].HasValue {
		t.Fatalf("HasValue: password=%v bank_card=%v", byPath["aa-1"].HasValue, byPath["aa-2"].HasValue)
	}
	all, err := p.List("")
	if err != nil || len(all) != 3 {
		t.Fatalf("full list: %v %d", err, len(all))
	}
}

func TestPersonaWriteDeleteUnsupported(t *testing.T) {
	f := newFakePersona(t)
	p, _ := NewPersona(personaCfg(f), 0, nil)
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	err := p.Write("some/path", &Entry{Value: "x"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("write: want ErrUnsupported, got %v", err)
	}
	if err := p.Delete("some/path"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("delete: want ErrUnsupported, got %v", err)
	}
	if n := f.reqs.Load(); n != 2 { // 仅 unlock 双探（health+items），写/删不碰网络
		t.Fatalf("write/delete must not hit the wire, requests=%d", n)
	}
}

func TestPersonaSessionTTLExpiry(t *testing.T) {
	f := newFakePersona(t)
	f.add("id-1", "one", "password", map[string]any{"Password": map[string]any{"password": "v"}})
	p, _ := NewPersona(personaCfg(f), time.Hour, nil)
	base := time.Unix(1700000000, 0)
	p.sess.nowFn = func() time.Time { return base }
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	p.sess.nowFn = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := p.Read("id-1"); !errors.Is(err, ErrLocked) {
		t.Fatalf("TTL expiry: want ErrLocked, got %v", err)
	}
	p.sess.nowFn = func() time.Time { return base } // 回拨无效：Check 已自动上锁
	if _, err := p.Read("id-1"); !errors.Is(err, ErrLocked) {
		t.Fatal("auto-lock must persist")
	}
	if err := p.Unlock(""); err != nil { // 重新握手解锁
		t.Fatal(err)
	}
	if e, err := p.Read("id-1"); err != nil || e.Value != "v" {
		t.Fatalf("after re-unlock: %v %+v", err, e)
	}
}

func TestPersonaAuditHook(t *testing.T) {
	f := newFakePersona(t)
	f.add("id-1", "one", "password", map[string]any{"Password": map[string]any{"password": "v"}})
	var mu sync.Mutex
	var got []string
	p, _ := NewPersona(personaCfg(f), 0, AuditFunc(func(action, path string, ok bool) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, action+"/"+path)
	}))
	if _, err := p.Read("id-1"); !errors.Is(err, ErrLocked) {
		t.Fatal("precondition: locked read")
	}
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read("id-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read("id-missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("precondition: missing read")
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"vault.unlock/", "vault.read/id-1", "vault.read/id-missing"}
	if len(got) != len(want) {
		t.Fatalf("audit events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audit[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

// TestPersonaWalkthrough 验收走查：Unlock→List→Read→Write→Lock 全链路
// 留证（todo「密码箱」验收①的测试输出证据；persona Write 为显式 unsupported）。
func TestPersonaWalkthrough(t *testing.T) {
	f := newFakePersona(t)
	const id = "9f1c3b2a-0000-4000-8000-000000000001"
	f.add(id, "nas root", "password", map[string]any{"Password": map[string]any{"password": "s3cret-value"}})
	p, err := NewPersona(map[string]string{
		"endpoint": f.srv.URL, "token": f.token, "timeout": "3s",
	}, 30*time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("step1 Unlock: passphrase ignored (token is the credential)")
	if err := p.Unlock(""); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	t.Logf("step2 List: prefix-filtered metadata, no plaintext")
	metas, err := p.List("9f1c")
	if err != nil || len(metas) != 1 {
		t.Fatalf("List: %v %d metas", err, len(metas))
	}
	t.Logf("  -> path=%s kind=%s hasValue=%v", metas[0].Path, metas[0].Kind, metas[0].HasValue)
	t.Logf("step3 Read: %s", id)
	e, err := p.Read(id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	t.Logf("  -> kind=%s username=%q value=%q title=%q", e.Kind, e.Username, e.Value, e.Meta["persona_title"])
	if e.Value != "s3cret-value" {
		t.Fatalf("roundtrip value mismatch: %q", e.Value)
	}
	t.Logf("step4 Write (persona API is read-only):")
	err = p.Write(id, &Entry{Path: id, Value: "nope"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Write: want ErrUnsupported, got %v", err)
	}
	t.Logf("  -> explicit ErrUnsupported: %v", err)
	p.Lock()
	t.Logf("step5 Lock: local session closed; Read now fails with ErrLocked")
	if _, err := p.Read(id); !errors.Is(err, ErrLocked) {
		t.Fatalf("post-lock Read: %v", err)
	}
	t.Logf("walkthrough complete: unlock/list/read/write/lock exercised against fake persona")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPersonaCallErrorPaths(t *testing.T) {
	t.Run("bad endpoint url", func(t *testing.T) {
		// endpoint 通过 http(s) 前缀校验但 URL 非法 → 构造请求失败
		p, err := NewPersona(map[string]string{"endpoint": "http://bad host", "token": "t"}, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Unlock(""); !errors.Is(err, ErrLocked) {
			t.Fatalf("want ErrLocked, got %v", err)
		}
	})
	t.Run("body read error", func(t *testing.T) {
		f := newFakePersona(t)
		p, _ := NewPersona(personaCfg(f), 0, nil)
		p.client = &http.Client{Transport: rtFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(errReader{})}, nil
		})}
		if err := p.Unlock(""); !errors.Is(err, ErrLocked) {
			t.Fatalf("want ErrLocked, got %v", err)
		}
	})
	t.Run("rate limited 429 is not locked", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			personaErr(w, http.StatusTooManyRequests, "rate_limited", "slow down")
		}))
		defer srv.Close()
		p, _ := NewPersona(map[string]string{"endpoint": srv.URL, "token": "t"}, 0, nil)
		err := p.Unlock("")
		if err == nil || errors.Is(err, ErrLocked) || !strings.Contains(err.Error(), "rate limited") {
			t.Fatalf("want explicit rate-limit error, got %v", err)
		}
	})
}

func TestPersonaDecodeErrorPaths(t *testing.T) {
	f := newFakePersona(t)
	f.add("id-ok", "ok", "password", map[string]any{"Password": map[string]any{"password": "v"}})
	f.addRaw("id-raw", json.RawMessage("42")) // 详情非对象 → 条目解码失败
	p, _ := NewPersona(personaCfg(f), 0, nil)
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read("id-raw"); err == nil || !strings.Contains(err.Error(), "decode item") {
		t.Fatalf("want item decode error, got %v", err)
	}
	f.mu.Lock()
	f.poison = true
	f.mu.Unlock()
	if _, err := p.List(""); err == nil || !strings.Contains(err.Error(), "decode items") {
		t.Fatalf("want list decode error, got %v", err)
	}
	f.mu.Lock()
	f.poison = false
	f.mu.Unlock()
	// 服务端中途失联 → 列表上报类错误（会话门已过，错误经 HTTP 分类）
	f.srv.Close()
	if _, err := p.List(""); !errors.Is(err, ErrLocked) {
		t.Fatalf("want ErrLocked after server down, got %v", err)
	}
	// 参数校验：Write/Delete 空 path（会话已解锁，走 path 校验分支）
	if err := p.Write("", &Entry{Value: "x"}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("write empty path: %v", err)
	}
	if err := p.Delete(""); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("delete empty path: %v", err)
	}
}

func TestPersonaExtractSecretShapes(t *testing.T) {
	if _, _, err := personaExtractSecret("password", json.RawMessage("42")); err == nil || !strings.Contains(err.Error(), "decode data") {
		t.Fatalf("bad data: %v", err)
	}
	if _, _, err := personaExtractSecret("password", json.RawMessage("{}")); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("empty variants: %v", err)
	}
	if _, _, err := personaExtractSecret("password", json.RawMessage(`{"A": {}, "B": {}}`)); err == nil {
		t.Fatal("multi variant should fail")
	}
	if _, _, err := personaExtractSecret("password", json.RawMessage(`{"Password": 42}`)); err == nil || !strings.Contains(err.Error(), "decode Password") {
		t.Fatalf("payload decode: %v", err)
	}
	if _, _, err := personaExtractSecret("password", json.RawMessage(`{"Password": {"password": 42}}`)); err == nil || !strings.Contains(err.Error(), "decode Password.password") {
		t.Fatalf("value decode: %v", err)
	}
	if _, _, err := personaExtractSecret("bank_card", json.RawMessage(`{"BankCard": 42}`)); err == nil {
		t.Fatal("no-secret payload decode should fail")
	}
	// null / 空 data → 无秘密、无清单、不报错
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), {}} {
		v, fields, err := personaExtractSecret("password", raw)
		if err != nil || v != "" || len(fields) != 0 {
			t.Fatalf("empty data %v: %q %v %v", raw, v, fields, err)
		}
	}
	v, _, err := personaExtractSecret("password", json.RawMessage(`{"Password": {"password": "pw"}}`))
	if err != nil || v != "pw" {
		t.Fatalf("secret extract: %q %v", v, err)
	}
	// server_config.password 缺失（可空字段）→ 空值不报错
	v, _, err = personaExtractSecret("server_config", json.RawMessage(`{"ServerConfig": {"host": "h"}}`))
	if err != nil || v != "" {
		t.Fatalf("optional secret: %q %v", v, err)
	}
}

// TestPersonaRegistryFactory 经注册表端到端（vault.New("persona", cfg)），
// 证明 Factory(cfg map[string]string) 装配口径可用。
func TestPersonaRegistryFactory(t *testing.T) {
	f := newFakePersona(t)
	f.add("id-9", "nine", "api_key", map[string]any{"ApiKey": map[string]any{"api_key": "k9"}})
	Register("persona", func(cfg map[string]string) (Provider, error) {
		return NewPersona(cfg, 0, nil)
	})
	p, err := New("persona", personaCfg(f))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "persona" {
		t.Fatalf("name: %q", p.Name())
	}
	if err := p.Unlock(""); err != nil {
		t.Fatal(err)
	}
	e, err := p.Read("id-9")
	if err != nil || e.Value != "k9" {
		t.Fatalf("registry read: %v %+v", err, e)
	}
	// 缺 endpoint 经注册表同样 fail-fast
	if _, err := New("persona", map[string]string{"token": "t"}); err == nil {
		t.Fatal("want endpoint error via registry")
	}
}
