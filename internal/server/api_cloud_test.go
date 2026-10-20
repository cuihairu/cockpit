package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
