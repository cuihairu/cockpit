package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

// cloudStubVault 可注入 Write/Delete 错误的 vault 假件，覆盖
// handleCloudAccounts/handleCloudAccount 的 secret 读写错误分支。
type cloudStubVault struct {
	writeErr  error
	deleteErr error
}

func (f *cloudStubVault) Name() string        { return "cloud-stub" }
func (f *cloudStubVault) Unlock(string) error { return nil }
func (f *cloudStubVault) Lock()               {}
func (f *cloudStubVault) Read(string) (*vault.Entry, error) {
	return nil, vault.ErrNotFound
}
func (f *cloudStubVault) Write(string, *vault.Entry) error { return f.writeErr }
func (f *cloudStubVault) List(string) ([]vault.EntryMeta, error) {
	return nil, vault.ErrUnsupported
}
func (f *cloudStubVault) Delete(string) error { return f.deleteErr }

const covCloudSaveBody = `{"vendor":"aliyun","alias":"prod","secret":"x","confirm_key":"aliyun/prod"}`

// TestCovCloudAccountValidationBranches 校验/解码/方法面错误分支。
func TestCovCloudAccountValidationBranches(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	covWantCode(t, "invalid vendor case", callCloudAPI(s, http.MethodPut, "/api/cloud/accounts",
		`{"vendor":"Ali","alias":"prod","secret":"x","confirm_key":"Ali/prod"}`), http.StatusBadRequest)
	covWantCode(t, "secret required", callCloudAPI(s, http.MethodPut, "/api/cloud/accounts",
		`{"vendor":"aliyun","alias":"prod","confirm_key":"aliyun/prod"}`), http.StatusBadRequest)
	covWantCode(t, "invalid json body", callCloudAPI(s, http.MethodPut, "/api/cloud/accounts", `{`), http.StatusBadRequest)
	covWantCode(t, "method not allowed", callCloudAPI(s, http.MethodPost, "/api/cloud/accounts", `{}`), http.StatusMethodNotAllowed)
}

// TestCovCloudAccountVaultErrorBranches vault 写入错误面：通用错误 500、
// ErrUnsupported 501、无 provider、无 audit。
func TestCovCloudAccountVaultErrorBranches(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.vaultProvider = &cloudStubVault{writeErr: errors.New("vault write boom")}
	covWantCode(t, "write generic error", callCloudAPI(s, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)

	su, _ := newCloudAPIServer(t)
	su.vaultProvider = &cloudStubVault{writeErr: vault.ErrUnsupported}
	covWantCode(t, "write unsupported", callCloudAPI(su, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusNotImplemented)

	sn, _ := newCloudAPIServer(t)
	sn.vaultProvider = nil
	covWantCode(t, "nil vault provider", callCloudAPI(sn, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)

	sa, _ := newCloudAPIServer(t)
	sa.vaultProvider = nil
	sa.audit = nil
	covWantCode(t, "nil audit", callCloudAPI(sa, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)
}

// TestCovCloudAccountDBErrorBranches 存储面错误分支：list 失败、upsert
// 失败、upsert 后回查失败/查无、Scopes nil 归一。
func TestCovCloudAccountDBErrorBranches(t *testing.T) {
	sl, _ := newCloudAPIServer(t)
	sl.vaultProvider = &cloudStubVault{}
	covCloseDB(t, sl)
	covWantCode(t, "list closed db", callCloudAPI(sl, http.MethodGet, "/api/cloud/accounts", ""),
		http.StatusInternalServerError)

	su, _ := newCloudAPIServer(t)
	su.vaultProvider = &cloudStubVault{}
	if err := su.db.Session().Exec(
		"CREATE TRIGGER block_cloud_insert BEFORE INSERT ON cloud_accounts BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatalf("create insert trigger: %v", err)
	}
	covWantCode(t, "upsert blocked", callCloudAPI(su, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)

	// 坏行（scopes 非法 JSON）只影响回查全量 list：upsert 的 First 按
	// vendor+alias 过滤不扫到它，回查 ListCloudAccounts 扫描时报错。
	sp, _ := newCloudAPIServer(t)
	sp.vaultProvider = &cloudStubVault{}
	if err := sp.db.Session().Exec(
		`INSERT INTO cloud_accounts (id, vendor, alias, credential_ref, scopes, enabled, created_at, updated_at)
		 VALUES ('poison0001','poison','p','${X}','not-json',1,'2026-01-01','2026-01-01')`).Error; err != nil {
		t.Fatalf("seed poison row: %v", err)
	}
	covWantCode(t, "reload after upsert", callCloudAPI(sp, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)

	// AFTER INSERT 删行走空回查 → "Saved cloud account not found"。
	sf, _ := newCloudAPIServer(t)
	sf.vaultProvider = &cloudStubVault{}
	if err := sf.db.Session().Exec(
		"CREATE TRIGGER drop_cloud_insert AFTER INSERT ON cloud_accounts BEGIN DELETE FROM cloud_accounts; END").Error; err != nil {
		t.Fatalf("create delete-on-insert trigger: %v", err)
	}
	covWantCode(t, "saved not found", callCloudAPI(sf, http.MethodPut, "/api/cloud/accounts", covCloudSaveBody),
		http.StatusInternalServerError)
}

// TestCovCloudAccountDeleteBranches DELETE 面错误分支。
func TestCovCloudAccountDeleteBranches(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	s.vaultProvider = &cloudStubVault{}
	covWantCode(t, "get item method", callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/x", ""),
		http.StatusMethodNotAllowed)
	covWantCode(t, "empty id", callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/", `{}`),
		http.StatusBadRequest)
	covWantCode(t, "slash in id", callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/a/b", `{}`),
		http.StatusBadRequest)
	covWantCode(t, "not found", callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/nope", `{"confirm_id":"nope"}`),
		http.StatusNotFound)

	sl, _ := newCloudAPIServer(t)
	sl.vaultProvider = &cloudStubVault{}
	covCloseDB(t, sl)
	covWantCode(t, "delete closed db", callCloudAPI(sl, http.MethodDelete, "/api/cloud/accounts/x", `{"confirm_id":"x"}`),
		http.StatusInternalServerError)

	seedCloudAccount := func(t *testing.T, srv *Server) string {
		t.Helper()
		if err := srv.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
			t.Fatalf("seed account: %v", err)
		}
		accounts, err := srv.db.ListCloudAccounts()
		if err != nil || len(accounts) != 1 {
			t.Fatalf("accounts = %+v, %v", accounts, err)
		}
		return accounts[0].ID
	}

	sb, _ := newCloudAPIServer(t)
	sb.vaultProvider = &cloudStubVault{}
	idB := seedCloudAccount(t, sb)
	if err := sb.db.Session().Exec(
		"CREATE TRIGGER block_cloud_delete BEFORE DELETE ON cloud_accounts BEGIN SELECT RAISE(ABORT, 'blocked'); END").Error; err != nil {
		t.Fatalf("create delete trigger: %v", err)
	}
	covWantCode(t, "delete blocked", callCloudAPI(sb, http.MethodDelete, "/api/cloud/accounts/"+idB,
		`{"confirm_id":"`+idB+`"}`), http.StatusInternalServerError)

	sd, _ := newCloudAPIServer(t)
	sd.vaultProvider = &cloudStubVault{deleteErr: errors.New("vault delete boom")}
	idD := seedCloudAccount(t, sd)
	covWantCode(t, "secret delete error", callCloudAPI(sd, http.MethodDelete, "/api/cloud/accounts/"+idD,
		`{"confirm_id":"`+idD+`"}`), http.StatusInternalServerError)

	view := cloudAccountToView(&storage.CloudAccount{ID: "x"})
	if view.Scopes == nil || len(view.Scopes) != 0 {
		t.Fatalf("nil scopes should normalize to empty slice, got %+v", view.Scopes)
	}
}

// TestCovDiscoveryErrorTextFallback 非 context 错误走原文分支。
func TestCovDiscoveryErrorTextFallback(t *testing.T) {
	failures := discoveryErrors([]oob.ProbeResult{
		{Provider: "plain"},
		{Provider: "boom", Err: errors.New("probe exploded")},
	})
	if len(failures) != 1 || failures[0].Provider != "boom" || failures[0].Error != "probe exploded" {
		t.Fatalf("failures = %+v", failures)
	}
}
