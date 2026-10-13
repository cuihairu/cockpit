package server

// persona provider 接线测试：经 initVault 走真 HTTP（假 persona 服务端），
// 且切换回 local 不破坏既有 vault_entries 存量（provider 只改配置不改存量）。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

const vaultPersonaTestKey = "vault-server-persona-test-key-0123456789!"

func openVaultTestDB(t *testing.T) *storage.DB {
	t.Helper()
	vault.ApplyKey(vaultPersonaTestKey)
	db, err := storage.Open(storage.Config{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// fakePersonaSrv 与 vault 包测试同形的最小假 persona 服务端（httptest）。
func fakePersonaSrv(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/connect/health" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"status": "ok"}})
			return
		}
		if r.URL.Path == "/api/v1/connect/items" {
			if r.Header.Get("Authorization") != "Bearer pconn_ok" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "unauthorized"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": []any{
				map[string]any{"id": "item-1", "title": "nas", "type": "password", "urls": []string{}, "updated_at": "2026-10-13T00:00:00Z"},
			}})
			return
		}
		if r.URL.Path == "/api/v1/connect/items/item-1" {
			if r.Header.Get("Authorization") != "Bearer pconn_ok" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "unauthorized"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{
				"id": "item-1", "title": "nas", "type": "password", "username": "root",
				"urls": []string{}, "updated_at": "2026-10-13T00:00:00Z",
				"data": map[string]any{"Password": map[string]any{"password": "srv-pw"}},
			}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"code": "not_found"}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInitVaultPersonaEndToEnd(t *testing.T) {
	db := openVaultTestDB(t)
	srv := fakePersonaSrv(t)

	p, err := initVault(&config.VaultConfig{
		Provider: "persona",
		Providers: &config.VaultProvidersConfig{
			Persona: &config.PersonaVaultConfig{
				Endpoint: srv.URL, Token: "pconn_ok", Timeout: 2e9, SessionTTL: 5 * time.Minute,
			},
		},
	}, db, nil)
	if err != nil {
		t.Fatalf("initVault(persona): %v", err)
	}
	if p.Name() != "persona" {
		t.Fatalf("name = %q", p.Name())
	}
	if err := p.Unlock(""); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	e, err := p.Read("item-1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if e.Value != "srv-pw" || e.Username != "root" || e.Kind != "password" {
		t.Fatalf("entry: %+v", e)
	}
	if err := p.Write("item-1", &vault.Entry{Value: "x"}); !errors.Is(err, vault.ErrUnsupported) {
		t.Fatalf("Write: want ErrUnsupported, got %v", err)
	}
	p.Lock()
	if _, err := p.Read("item-1"); !errors.Is(err, vault.ErrLocked) {
		t.Fatalf("post-lock Read: %v", err)
	}
}

func TestInitVaultPersonaFailFast(t *testing.T) {
	db := openVaultTestDB(t)
	// 缺 endpoint：provider=persona 选中时启动即报错（fail-fast 不降级 local）
	_, err := initVault(&config.VaultConfig{
		Provider:  "persona",
		Providers: &config.VaultProvidersConfig{Persona: &config.PersonaVaultConfig{Token: "pconn_ok"}},
	}, db, nil)
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("want endpoint error, got %v", err)
	}
	// 缺 token 同样 fail-fast
	_, err = initVault(&config.VaultConfig{
		Provider:  "persona",
		Providers: &config.VaultProvidersConfig{Persona: &config.PersonaVaultConfig{Endpoint: "http://127.0.0.1:9"}},
	}, db, nil)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("want token error, got %v", err)
	}
	// local 选中时未配置的 persona 节不校验（不阻断启动）
	p, err := initVault(&config.VaultConfig{Provider: "local"}, db, nil)
	if err != nil || p.Name() != "local" {
		t.Fatalf("local with unconfigured persona section must start: %v %v", err, p)
	}
}

func TestInitVaultProviderSwitchKeepsLocalData(t *testing.T) {
	db := openVaultTestDB(t)
	// 先以 local 写入存量
	lp, err := initVault(&config.VaultConfig{Provider: "local"}, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lp.Write("keep/me", &vault.Entry{Kind: "password", Username: "u1", Value: "v1"}); err != nil {
		t.Fatal(err)
	}
	// 切到 persona：不再动库（本用例无服务端，只验证不报错与写拒绝）
	srv := fakePersonaSrv(t)
	pp, err := initVault(&config.VaultConfig{
		Provider:  "persona",
		Providers: &config.VaultProvidersConfig{Persona: &config.PersonaVaultConfig{Endpoint: srv.URL, Token: "pconn_ok"}},
	}, db, nil)
	if err != nil {
		t.Fatalf("initVault(persona): %v", err)
	}
	if err := pp.Unlock(""); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	// 切回 local：原存量完好
	lp2, err := initVault(&config.VaultConfig{Provider: "local"}, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := lp2.Read("keep/me")
	if err != nil || e.Value != "v1" || e.Username != "u1" {
		t.Fatalf("local data must survive provider switch: %v %+v", err, e)
	}
}

func TestInitVaultPersonaAuditHook(t *testing.T) {
	db := openVaultTestDB(t)
	srv := fakePersonaSrv(t)
	logger := audit.NewLogger(db)
	p, err := initVault(&config.VaultConfig{
		Provider:  "persona",
		Providers: &config.VaultProvidersConfig{Persona: &config.PersonaVaultConfig{Endpoint: srv.URL, Token: "pconn_ok"}},
	}, db, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Unlock(""); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if _, err := p.Read("item-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read("item-nope"); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("missing read: %v", err)
	}
	// 审计落 audit_logs（hook 由 initVault 注入）
	var n int64
	if err := db.Gorm().Raw("SELECT COUNT(*) FROM audit_logs WHERE resource = 'vault'").Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("persona ops must leave audit entries")
	}
}

func TestVaultEntriesAPIPersonaUnsupported(t *testing.T) {
	s := newTestServerWithDB(t)
	resetVaultTokens(t)
	vault.ApplyKey(vaultPersonaTestKey)
	srv := fakePersonaSrv(t)
	p, err := initVault(&config.VaultConfig{
		Provider:  "persona",
		Providers: &config.VaultProvidersConfig{Persona: &config.PersonaVaultConfig{Endpoint: srv.URL, Token: "pconn_ok"}},
	}, s.db, nil)
	if err != nil {
		t.Fatalf("initVault: %v", err)
	}
	if err := p.Unlock(""); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	s.vaultProvider = p
	token := vaultVerifyLogin(t, s)

	// PUT → 501（persona 只读，显式 unsupported 而非 500）
	req := authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodPut, "/api/remote/vault/entries/nas/api1", strings.NewReader(`{"kind":"pair","username":"u","value":"v"}`)))
	req.Header.Set("X-Vault-Token", token)
	rec := httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("put status = %d, want 501, body=%s", rec.Code, rec.Body.String())
	}
	// DELETE → 501
	req = authenticateAdminRequest(t, s, httptest.NewRequest(http.MethodDelete, "/api/remote/vault/entries/nas/api1", nil))
	req.Header.Set("X-Vault-Token", token)
	rec = httptest.NewRecorder()
	s.authService().Middleware(s.handleVaultEntry)(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("delete status = %d, want 501, body=%s", rec.Code, rec.Body.String())
	}
}
