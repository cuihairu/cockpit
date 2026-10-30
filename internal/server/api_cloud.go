package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/cuihairu/cockpit/internal/vault"
)

// 多云账号接入 API（cloud-provider.md 片2）。台账只存并只回 ${VAR} 引用；
// secret 通过 cloud/{vendor}/{alias} 落既有 vault provider，凭据明文不进
// 数据库、API 响应或审计。
type cloudAccountRequest struct {
	Vendor     string   `json:"vendor"`
	Alias      string   `json:"alias"`
	Secret     string   `json:"secret"`
	Username   string   `json:"username,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	Enabled    *bool    `json:"enabled,omitempty"`
	ConfirmKey string   `json:"confirm_key,omitempty"`
}

type cloudAccountView struct {
	ID            string   `json:"id"`
	Vendor        string   `json:"vendor"`
	Alias         string   `json:"alias"`
	CredentialRef string   `json:"credentialRef"`
	Scopes        []string `json:"scopes"`
	Enabled       bool     `json:"enabled"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

// cloudInstanceView 是 instances 端点对外视图，与 accounts 端点同口径：
// 时间戳统一序列化为 "2006-01-02T15:04:05Z07:00"（无亚秒），避免与
// storage.CloudInstance 的 time.Time（含亚秒）格式不一致。
type cloudInstanceView struct {
	ID               string `json:"id"`
	AccountID        string `json:"accountId"`
	NativeID         string `json:"nativeId"`
	Name             string `json:"name"`
	Region           string `json:"region"`
	Zone             string `json:"zone,omitempty"`
	Spec             string `json:"spec,omitempty"`
	Status           string `json:"status"`
	BillingMode      string `json:"billingMode"`
	BillingContinues bool   `json:"billingContinues"`
	RawVendorStatus  string `json:"rawVendorStatus,omitempty"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

var cloudAccountKeyRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)

func cloudVaultPath(vendor, alias string) string {
	return "cloud/" + vendor + "/" + alias
}

func (req *cloudAccountRequest) validate() (string, bool) {
	if !cloudAccountKeyRe.MatchString(req.Vendor) || !cloudAccountKeyRe.MatchString(req.Alias) {
		return "vendor and alias must be lowercase identifiers", false
	}
	if req.Secret == "" {
		return "secret required", false
	}
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}
	return "", true
}

func cloudAccountToView(account *storage.CloudAccount) cloudAccountView {
	if account.Scopes == nil {
		account.Scopes = []string{}
	}
	return cloudAccountView{
		ID:            account.ID,
		Vendor:        account.Vendor,
		Alias:         account.Alias,
		CredentialRef: account.CredentialRef,
		Scopes:        account.Scopes,
		Enabled:       account.Enabled,
		CreatedAt:     account.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     account.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func cloudInstanceToView(inst *storage.CloudInstance) cloudInstanceView {
	return cloudInstanceView{
		ID:               inst.ID,
		AccountID:        inst.AccountID,
		NativeID:         inst.NativeID,
		Name:             inst.Name,
		Region:           inst.Region,
		Zone:             inst.Zone,
		Spec:             inst.Spec,
		Status:           inst.Status,
		BillingMode:      inst.BillingMode,
		BillingContinues: inst.BillingContinues,
		RawVendorStatus:  inst.RawVendorStatus,
		CreatedAt:        inst.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:        inst.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *Server) registerCloudAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/cloud/accounts", s.authService().Middleware(s.handleCloudAccounts))
	mux.HandleFunc("/api/cloud/accounts/", s.authService().Middleware(s.handleCloudAccount))
}

// handleCloudInstances GET /api/cloud/accounts/{id}/instances
// 只读实例台账（发现接线的最小切片）：按账号回读已落库的云实例，不触
// 厂商 API、不读 secret。账号不存在回 404；空台账回空数组。
func (s *Server) handleCloudInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/cloud/accounts/")
	// 统一归一化：去 "/instances" 后缀与首尾 "/"，使尾斜杠路径
	// /api/cloud/accounts/{id}/instances/ 与无尾斜杠等价。
	id := strings.Trim(strings.TrimSuffix(strings.TrimSuffix(rest, "/"), "/instances"), "/")
	if id == "" || strings.Contains(id, "/") {
		s.handleError(w, r, http.StatusBadRequest, "cloud account id required")
		return
	}

	accounts, err := s.db.ListCloudAccounts()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load cloud account")
		return
	}
	found := false
	for i := range accounts {
		if accounts[i].ID == id {
			found = true
			break
		}
	}
	if !found {
		s.handleError(w, r, http.StatusNotFound, "Cloud account not found")
		return
	}

	instances, err := s.db.ListCloudInstances(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list cloud instances")
		return
	}
	views := make([]cloudInstanceView, 0, len(instances))
	for i := range instances {
		views = append(views, cloudInstanceToView(&instances[i]))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": views})
}

// handleCloudAccounts GET 台账 / PUT 保存账号与 secret 引用。
func (s *Server) handleCloudAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		accounts, err := s.db.ListCloudAccounts()
		if err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to list cloud accounts")
			return
		}
		views := make([]cloudAccountView, 0, len(accounts))
		for i := range accounts {
			views = append(views, cloudAccountToView(&accounts[i]))
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": views})

	case http.MethodPut:
		var req cloudAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if message, ok := req.validate(); !ok {
			s.handleError(w, r, http.StatusBadRequest, message)
			return
		}
		if req.ConfirmKey != req.Vendor+"/"+req.Alias {
			s.handleError(w, r, http.StatusBadRequest, "confirm_key must exactly match vendor/alias")
			return
		}
		if err := s.writeCloudSecret(r, &req); err != nil {
			s.handleCloudSecretError(w, r, req.Vendor+"/"+req.Alias, err, "write")
			return
		}
		ref := "${CLOUD_" + strings.ToUpper(req.Vendor) + "_" + strings.ToUpper(req.Alias) + "}"
		input := &storage.CloudAccountInput{
			Vendor: req.Vendor, Alias: req.Alias, CredentialRef: ref,
			Scopes: req.Scopes, Enabled: *req.Enabled,
		}
		if err := s.db.UpsertCloudAccount(input); err != nil {
			s.auditCloudAccount(r, "cloud_account_update", req.Vendor+"/"+req.Alias, false, map[string]string{"reason": "persist failed"})
			s.handleError(w, r, http.StatusInternalServerError, "Failed to save cloud account")
			return
		}
		accounts, err := s.db.ListCloudAccounts()
		if err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to load cloud account")
			return
		}
		for i := range accounts {
			if accounts[i].Vendor == req.Vendor && accounts[i].Alias == req.Alias {
				s.auditCloudAccount(r, "cloud_account_update", req.Vendor+"/"+req.Alias, true, map[string]string{"credential_ref": ref})
				s.writeJSON(w, http.StatusOK, cloudAccountToView(&accounts[i]))
				return
			}
		}
		s.handleError(w, r, http.StatusInternalServerError, "Saved cloud account not found")

	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleCloudAccount DELETE /api/cloud/accounts/{id}；confirm_id 必须逐字一致。
func (s *Server) handleCloudAccount(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/instances") {
		s.handleCloudInstances(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cloud/accounts/"), "/")
	if id == "" || strings.Contains(id, "/") {
		s.handleError(w, r, http.StatusBadRequest, "cloud account id required")
		return
	}
	var req struct {
		ConfirmID string `json:"confirm_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ConfirmID != id {
		s.handleError(w, r, http.StatusBadRequest, "confirm_id must exactly match account id")
		return
	}

	accounts, err := s.db.ListCloudAccounts()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load cloud account")
		return
	}
	var found *storage.CloudAccount
	for i := range accounts {
		if accounts[i].ID == id {
			found = &accounts[i]
			break
		}
	}
	if found == nil {
		s.handleError(w, r, http.StatusNotFound, "Cloud account not found")
		return
	}
	if err := s.db.DeleteCloudAccount(id); err != nil {
		s.auditCloudAccount(r, "cloud_account_delete", found.Vendor+"/"+found.Alias, false, map[string]string{"reason": "persist failed"})
		s.handleError(w, r, http.StatusInternalServerError, "Failed to delete cloud account")
		return
	}
	if s.vaultProvider != nil {
		if err := s.vaultProvider.Delete(cloudVaultPath(found.Vendor, found.Alias)); err != nil &&
			!errors.Is(err, vault.ErrNotFound) && !errors.Is(err, vault.ErrUnsupported) {
			s.auditCloudAccount(r, "cloud_account_delete", found.Vendor+"/"+found.Alias, false, map[string]string{"reason": "secret delete failed"})
			s.handleError(w, r, http.StatusInternalServerError, "Failed to delete cloud secret")
			return
		}
	}
	s.auditCloudAccount(r, "cloud_account_delete", found.Vendor+"/"+found.Alias, true, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) writeCloudSecret(r *http.Request, req *cloudAccountRequest) error {
	if s.vaultProvider == nil {
		return vault.ErrNotFound
	}
	entry := &vault.Entry{
		Path:     cloudVaultPath(req.Vendor, req.Alias),
		Kind:     "api_token",
		Username: req.Username,
		Value:    req.Secret,
		Meta:     map[string]string{"vendor": req.Vendor, "alias": req.Alias},
	}
	return s.vaultProvider.Write(entry.Path, entry)
}

func (s *Server) handleCloudSecretError(w http.ResponseWriter, r *http.Request, key string, err error, action string) {
	details := map[string]string{"reason": action + " failed"}
	status := http.StatusInternalServerError
	if errors.Is(err, vault.ErrUnsupported) {
		status = http.StatusNotImplemented
		details["reason"] = "unsupported by vault provider"
	}
	s.auditCloudAccount(r, "cloud_account_update", key, false, details)
	s.handleError(w, r, status, "Failed to store cloud secret")
}

func (s *Server) auditCloudAccount(r *http.Request, action, key string, success bool, details map[string]string) {
	if s.audit == nil {
		return
	}
	status := audit.StatusSuccess
	if !success {
		status = audit.StatusFailure
	}
	userID, username := "", ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		userID, username = userInfo.UserID, userInfo.Username
	}
	_ = s.audit.Log(&audit.LogEntry{
		UserID: userID, Username: username, Action: action, Status: status,
		Resource: audit.ResourceCloudAccount, ResourceID: key, Details: details,
		IP: s.getClientIP(r), UserAgent: r.UserAgent(),
	})
}
