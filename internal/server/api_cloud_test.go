package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

func newCloudAPIServer(t *testing.T) (*Server, vault.Provider) {
	t.Helper()
	vault.ApplyKey("cloud-api-test-key-0123456789abcdef!")
	s := newTestServerWithDB(t)
	provider, err := initVault(&config.VaultConfig{}, s.db, s.audit)
	if err != nil {
		t.Fatalf("initVault() error = %v", err)
	}
	s.vaultProvider = provider
	return s, provider
}

func callCloudAPI(s *Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	token, _ := s.auth.GenerateToken("1", "admin", "admin")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.authService().Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path {
		case "/api/cloud/accounts":
			s.handleCloudAccounts(w, r)
		default:
			s.handleCloudAccount(w, r)
		}
	}))(rec, req)
	return rec
}

func TestCloudAccountsLifecycle(t *testing.T) {
	s, vaultProvider := newCloudAPIServer(t)

	save := `{"vendor":"aliyun","alias":"prod","secret":"secret-value","scopes":["ecs:read"],"confirm_key":"aliyun/prod"}`
	if code := callCloudAPI(s, http.MethodPut, "/api/cloud/accounts", save).Code; code != http.StatusOK {
		t.Fatalf("save status = %d, want %d", code, http.StatusOK)
	}
	entry, err := vaultProvider.Read("cloud/aliyun/prod")
	if err != nil || entry.Value != "secret-value" {
		t.Fatalf("secret entry = %+v, %v", entry, err)
	}

	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-value") || !strings.Contains(rec.Body.String(), "${CLOUD_ALIYUN_PROD}") {
		t.Fatalf("list leaked secret or omitted ref: %s", rec.Body.String())
	}

	accounts, err := s.db.ListCloudAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	id := accounts[0].ID
	if code := callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/"+id, `{"confirm_id":"`+id+`"}`).Code; code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
	if _, err := vaultProvider.Read("cloud/aliyun/prod"); err == nil {
		t.Fatal("secret should be deleted with account")
	}
}

func TestCloudAccountsDoubleConfirmation(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if code := callCloudAPI(s, http.MethodPut, "/api/cloud/accounts", `{"vendor":"aliyun","alias":"prod","secret":"x","confirm_key":"aliyun/other"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("save confirm status = %d", code)
	}
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	if code := callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/"+accounts[0].ID, `{"confirm_id":"different"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("delete confirm status = %d", code)
	}
	if accounts, _ := s.db.ListCloudAccounts(); len(accounts) != 1 {
		t.Fatal("account should survive failed confirmation")
	}
}

func TestCloudInstancesReadOnlyLedger(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/nope/instances", "").Code; code != http.StatusNotFound {
		t.Fatalf("missing account status = %d", code)
	}

	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, err := s.db.ListCloudAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	accountID := accounts[0].ID
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: accountID, NativeID: "i-1", Name: "web-1", Region: "cn-hangzhou",
		Status: "running", BillingMode: "prepaid", BillingContinues: true,
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+accountID+"/instances", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data []storage.CloudInstance `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].NativeID != "i-1" || payload.Data[0].AccountID != accountID {
		t.Fatalf("instances = %+v", payload.Data)
	}

	if code := callCloudAPI(s, http.MethodDelete, "/api/cloud/accounts/"+accountID+"/instances", "").Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("write status = %d", code)
	}
}

func TestCloudInstancesEmptyLedger(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	rec := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+accounts[0].ID+"/instances", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("empty ledger response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCloudInstancesListError(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	// 账号表仍可读（ListCloudAccounts 通过），仅实例表不可查：DROP 表让
	// ListCloudInstances 报错，精确命中实例查询失败分支。
	if err := s.db.Session().Exec("DROP TABLE cloud_instances").Error; err != nil {
		t.Fatalf("drop cloud_instances: %v", err)
	}
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+accounts[0].ID+"/instances", "").Code; code != http.StatusInternalServerError {
		t.Fatalf("closed DB status = %d", code)
	}
}

func TestCloudInstancesAccountsError(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	covCloseDB(t, s)
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/"+accounts[0].ID+"/instances", "").Code; code != http.StatusInternalServerError {
		t.Fatalf("closed DB status = %d", code)
	}
}

func TestCloudInstancesBadPath(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts//instances", "").Code; code != http.StatusBadRequest {
		t.Fatalf("empty id status = %d", code)
	}
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts/a/b/instances", "").Code; code != http.StatusBadRequest {
		t.Fatalf("slash id status = %d", code)
	}
	if code := callCloudAPI(s, http.MethodGet, "/api/cloud/accounts", "").Code; code != http.StatusOK {
		t.Fatalf("plain ledger status = %d", code)
	}
}

func TestCloudInstancesRegisterRoute(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	mux := http.NewServeMux()
	s.registerCloudAPI(mux)
	token, _ := s.auth.GenerateToken("1", "admin", "admin")
	req := httptest.NewRequest(http.MethodGet, "/api/cloud/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("registered route status = %d", rec.Code)
	}
}

// TestCloudInstancesServeMuxRouting 经真实 ServeMux 验证 instances 端点
// 的路由分发：/instances 与 /instances/ 均可达且等价，转发判定在 mux 层生效。
func TestCloudInstancesServeMuxRouting(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	accountID := accounts[0].ID
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: accountID, NativeID: "i-1", Name: "web-1", Region: "cn-hangzhou",
		Status: "running", BillingMode: "prepaid",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	mux := http.NewServeMux()
	s.registerCloudAPI(mux)
	token, _ := s.auth.GenerateToken("1", "admin", "admin")

	for _, path := range []string{"/api/cloud/accounts/" + accountID + "/instances", "/api/cloud/accounts/" + accountID + "/instances/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("mux %s status = %d, body %s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"nativeId":"i-1"`) {
			t.Fatalf("mux %s missing instance: %s", path, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/accounts/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("root accounts status = %d, want 405", rec.Code)
	}
}

// TestCloudInstancesTimestampFormat 断言 instances 端点的时间戳序列化口径
// 与 accounts 端点一致：RFC3339 无亚秒（"2006-01-02T15:04:05Z07:00"），
// 而非原始 storage.CloudInstance 的 time.Time（JSON 含亚秒）。
func TestCloudInstancesTimestampFormat(t *testing.T) {
	s, _ := newCloudAPIServer(t)
	if err := s.db.UpsertCloudAccount(&storage.CloudAccountInput{Vendor: "aliyun", Alias: "prod", CredentialRef: "${AK}"}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	accounts, _ := s.db.ListCloudAccounts()
	accountID := accounts[0].ID
	if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
		AccountID: accountID, NativeID: "i-1", Name: "web-1", Region: "cn-hangzhou",
		Status: "running", BillingMode: "prepaid",
	}); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	mux := http.NewServeMux()
	s.registerCloudAPI(mux)
	token, _ := s.auth.GenerateToken("1", "admin", "admin")
	req := httptest.NewRequest(http.MethodGet, "/api/cloud/accounts/"+accountID+"/instances", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data []struct {
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("instances = %d, want 1", len(payload.Data))
	}
	for _, field := range []string{payload.Data[0].CreatedAt, payload.Data[0].UpdatedAt} {
		if strings.Contains(field, ".") {
			t.Fatalf("timestamp contains sub-second: %q", field)
		}
		if _, err := time.Parse("2006-01-02T15:04:05Z07:00", field); err != nil {
			t.Fatalf("timestamp not RFC3339-no-subsecond: %q (%v)", field, err)
		}
	}
}
