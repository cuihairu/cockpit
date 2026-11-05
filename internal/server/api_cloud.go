package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/cloud/aliyun"
	"github.com/cuihairu/cockpit/internal/cloud/tencent"
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
	Region     string   `json:"region,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	Enabled    *bool    `json:"enabled,omitempty"`
	ConfirmKey string   `json:"confirm_key,omitempty"`
}

type cloudAccountView struct {
	ID            string   `json:"id"`
	Vendor        string   `json:"vendor"`
	Alias         string   `json:"alias"`
	CredentialRef string   `json:"credentialRef"`
	Region        string   `json:"region,omitempty"`
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
		Region:        account.Region,
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

// cloudVaultCredentials 读 cloud/{vendor}/{alias} 凭据对：Username=AccessKey/ID，
// Value=SecretKey。适配器 New 的 nil 返回统一视为未配置。
func (s *Server) cloudVaultCredentials(account *storage.CloudAccount) (string, string, error) {
	if s.vaultProvider == nil {
		return "", "", vault.ErrNotFound
	}
	entry, err := s.vaultProvider.Read(cloudVaultPath(account.Vendor, account.Alias))
	if err != nil {
		return "", "", err
	}
	if entry.Username == "" || entry.Value == "" {
		return "", "", fmt.Errorf("vault cloud credential %s incomplete", cloudVaultPath(account.Vendor, account.Alias))
	}
	return entry.Username, entry.Value, nil
}

// cloudProvider 构造账号适配器；当前 P0 两家 fail-closed（未知厂商 501）。
func (s *Server) cloudProvider(account *storage.CloudAccount) (cloud.Provider, error) {
	if s.cloudProviderFn != nil {
		return s.cloudProviderFn(account)
	}
	switch account.Vendor {
	case "aliyun", "tencent":
	default:
		return nil, cloud.ErrNotSupported
	}
	id, secret, err := s.cloudVaultCredentials(account)
	if err != nil {
		return nil, err
	}
	if account.Vendor == "aliyun" {
		provider := aliyun.New(id, secret, account.Region)
		if provider == nil {
			return nil, fmt.Errorf("cloud provider %s not configured", account.Vendor)
		}
		return provider, nil
	}
	provider := tencent.New(id, secret, account.Region)
	if provider == nil {
		return nil, fmt.Errorf("cloud provider %s not configured", account.Vendor)
	}
	return provider, nil
}

// cloudProviderForAccount 按账号 ID 取账号并构造适配器。
func (s *Server) cloudProviderForAccount(id string) (*storage.CloudAccount, cloud.Provider, error) {
	account, err := s.db.GetCloudAccount(id)
	if err != nil {
		return nil, nil, err
	}
	provider, err := s.cloudProvider(&account)
	if err != nil {
		return nil, nil, err
	}
	return &account, provider, nil
}

// syncCloudInstances 调厂商 ListInstances 全量同步到 cloud_instances 台账。
// 覆盖式语义：不在厂商当前列表中的本地行保留（删除断点供人工核对），不自动删。
func (s *Server) syncCloudInstances(ctx context.Context, account *storage.CloudAccount, provider cloud.Provider) error {
	compute, ok := provider.(cloud.ComputeProvider)
	if !ok {
		return cloud.ErrNotSupported
	}
	instances, err := compute.ListInstances(ctx)
	if err != nil {
		return err
	}
	for i := range instances {
		it := instances[i]
		if err := s.db.UpsertCloudInstance(&storage.CloudInstanceInput{
			AccountID:        account.ID,
			NativeID:         it.NativeID,
			Name:             it.Name,
			Region:           it.Region,
			Zone:             it.Zone,
			Spec:             it.Spec,
			Status:           string(it.Status),
			BillingMode:      string(it.BillingMode),
			BillingContinues: it.BillingContinues,
			RawVendorStatus:  it.RawVendorStatus,
		}); err != nil {
			return err
		}
	}
	return nil
}

// cloudInstanceRequest 生命周期/创建/操作轮询请求。
type cloudInstanceRequest struct {
	InstanceID      string `json:"instance_id,omitempty"`
	OperationID     string `json:"operation_id,omitempty"`
	Name            string `json:"name"`
	Region          string `json:"region,omitempty"`
	Zone            string `json:"zone,omitempty"`
	Spec            string `json:"spec"`
	ImageID         string `json:"image_id"`
	SecurityGroupID string `json:"security_group_id,omitempty"`
	BillingMode     string `json:"billing_mode,omitempty"`
	Action          string `json:"action,omitempty"`
	ConfirmKey      string `json:"confirm_key,omitempty"`
}

// handleCloudInstancesRoot /api/cloud/instances/{id}/actions|operations。
func (s *Server) handleCloudInstancesRoot(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/cloud/instances/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		s.handleError(w, r, http.StatusBadRequest, "cloud instance route required")
		return
	}
	switch parts[1] {
	case "actions":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		s.handleCloudInstanceAction(w, r, parts[0])
	case "operations":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		s.handleCloudOperation(w, r, parts[0])
	default:
		s.handleError(w, r, http.StatusNotFound, "Not found")
	}
}

func (s *Server) handleCloudInstanceAction(w http.ResponseWriter, r *http.Request, accountID string) {
	var req cloudInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	account, provider, err := s.cloudProviderForAccount(accountID)
	if err != nil {
		s.handleCloudProviderError(w, r, err)
		return
	}
	compute, ok := provider.(cloud.ComputeProvider)
	if !ok {
		s.handleCloudProviderError(w, r, cloud.ErrNotSupported)
		return
	}
	switch req.Action {
	case "start", "stop", "delete":
		s.runCloudLifecycleAction(w, r, account, compute, req)
	case "create":
		s.runCloudCreateAction(w, r, account, compute, req)
	default:
		s.handleError(w, r, http.StatusBadRequest, "action must be create/start/stop/delete")
	}
}

func (s *Server) runCloudLifecycleAction(w http.ResponseWriter, r *http.Request, account *storage.CloudAccount, compute cloud.ComputeProvider, req cloudInstanceRequest) {
	if req.InstanceID == "" || req.ConfirmKey != req.InstanceID {
		s.handleError(w, r, http.StatusBadRequest, "instance_id required and confirm_key must exactly match instance_id")
		return
	}
	inst, err := s.db.GetCloudInstance(req.InstanceID)
	if err != nil {
		s.handleCloudProviderError(w, r, err)
		return
	}
	// 越权防护：实例必须属于请求的账号（台账行 account_id 硬校验）。
	if inst.AccountID != account.ID {
		s.handleError(w, r, http.StatusNotFound, "Cloud instance not found")
		return
	}
	var op cloud.Operation
	switch req.Action {
	case "start":
		op, err = compute.StartInstance(r.Context(), inst.NativeID)
	case "stop":
		op, err = compute.StopInstance(r.Context(), inst.NativeID)
	default:
		op, err = compute.DeleteInstance(r.Context(), inst.NativeID)
	}
	if err != nil {
		s.auditCloudInstance(r, "cloud_instance_action", account.Vendor+"/"+req.InstanceID, false, map[string]string{"reason": "vendor rejected"})
		s.handleError(w, r, http.StatusBadGateway, "Failed to call cloud provider")
		return
	}
	op.ID = fmt.Sprintf("%s-%d", op.ID, time.Now().UnixNano())
	s.storeCloudOperation(account.ID, &op)
	s.auditCloudInstance(r, "cloud_instance_action", account.Vendor+"/"+req.InstanceID, true, map[string]string{"action": req.Action})
	s.writeJSON(w, http.StatusAccepted, op)
}

func (s *Server) runCloudCreateAction(w http.ResponseWriter, r *http.Request, account *storage.CloudAccount, compute cloud.ComputeProvider, req cloudInstanceRequest) {
	billing := cloud.BillingOnDemand
	switch req.BillingMode {
	case "":
	case string(cloud.BillingPrepaid):
		billing = cloud.BillingPrepaid
	case string(cloud.BillingSpot):
		billing = cloud.BillingSpot
	default:
		s.handleError(w, r, http.StatusBadRequest, "billing_mode must be on-demand/prepaid/spot")
		return
	}
	op, err := compute.CreateInstance(r.Context(), cloud.InstanceSpec{
		Name: req.Name, Region: req.Region, Zone: req.Zone,
		Spec: req.Spec, ImageID: req.ImageID,
		SecurityGroupID: req.SecurityGroupID, BillingMode: billing,
	})
	if err != nil {
		s.auditCloudInstance(r, "cloud_instance_create", account.Vendor+"/"+req.Name, false, map[string]string{"reason": "vendor rejected"})
		s.handleError(w, r, http.StatusBadGateway, "Failed to call cloud provider")
		return
	}
	op.ID = fmt.Sprintf("%s-%d", op.ID, time.Now().UnixNano())
	s.storeCloudOperation(account.ID, &op)
	s.auditCloudInstance(r, "cloud_instance_create", account.Vendor+"/"+req.Name, true, nil)
	s.writeJSON(w, http.StatusAccepted, op)
}

func (s *Server) handleCloudOperation(w http.ResponseWriter, r *http.Request, accountID string) {
	var req cloudInstanceRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	_, provider, err := s.cloudProviderForAccount(accountID)
	if err != nil {
		s.handleCloudProviderError(w, r, err)
		return
	}
	compute, ok := provider.(cloud.ComputeProvider)
	if !ok {
		s.handleCloudProviderError(w, r, cloud.ErrNotSupported)
		return
	}
	if req.OperationID == "" {
		ops := s.listCloudOperations(accountID)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": ops})
		return
	}
	op, ok := s.getCloudOperation(accountID, req.OperationID)
	if !ok {
		s.handleError(w, r, http.StatusNotFound, "Operation not found")
		return
	}
	if op.IsTerminal() {
		s.writeJSON(w, http.StatusOK, op)
		return
	}
	advanced, err := compute.GetOperation(r.Context(), op)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "Failed to poll cloud provider")
		return
	}
	s.storeCloudOperation(accountID, &advanced)
	s.writeJSON(w, http.StatusOK, advanced)
}

func cloudOperationKey(accountID, operationID string) string {
	return accountID + "/" + operationID
}

func (s *Server) storeCloudOperation(accountID string, op *cloud.Operation) {
	s.cloudOpsMu.Lock()
	defer s.cloudOpsMu.Unlock()
	if s.cloudOps == nil {
		s.cloudOps = map[string]cloud.Operation{}
	}
	s.cloudOps[cloudOperationKey(accountID, op.ID)] = *op
}

func (s *Server) getCloudOperation(accountID, operationID string) (cloud.Operation, bool) {
	s.cloudOpsMu.Lock()
	defer s.cloudOpsMu.Unlock()
	op, ok := s.cloudOps[cloudOperationKey(accountID, operationID)]
	return op, ok
}

func (s *Server) listCloudOperations(accountID string) []cloud.Operation {
	s.cloudOpsMu.Lock()
	defer s.cloudOpsMu.Unlock()
	out := make([]cloud.Operation, 0, len(s.cloudOps))
	for _, op := range s.cloudOps {
		if strings.HasPrefix(cloudOperationKey(accountID, op.ID), accountID+"/") {
			out = append(out, op)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out
}

func (s *Server) handleCloudProviderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.handleError(w, r, http.StatusNotFound, "Cloud resource not found")
	case errors.Is(err, vault.ErrLocked):
		s.handleError(w, r, http.StatusServiceUnavailable, "Vault is locked")
	case errors.Is(err, vault.ErrNotFound):
		s.handleError(w, r, http.StatusServiceUnavailable, "Cloud credential not configured")
	case errors.Is(err, cloud.ErrNotSupported):
		s.handleError(w, r, http.StatusNotImplemented, "Cloud provider capability not supported")
	default:
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load cloud provider")
	}
}

func (s *Server) auditCloudInstance(r *http.Request, action, key string, success bool, details map[string]string) {
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
		Resource: audit.ResourceCloudInstance, ResourceID: key, Details: details,
		IP: s.getClientIP(r), UserAgent: r.UserAgent(),
	})
}

func (s *Server) registerCloudAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/cloud/accounts", s.authService().Middleware(s.handleCloudAccounts))
	mux.HandleFunc("/api/cloud/accounts/", s.authService().Middleware(s.handleCloudAccount))
	mux.HandleFunc("/api/cloud/instances/", s.authService().Middleware(s.handleCloudInstancesRoot))
}

// handleCloudInstances GET /api/cloud/accounts/{id}/instances
// 发现接线：账号存在且凭据/地域齐备时调厂商 ListInstances 全量同步
// cloud_instances 台账，再回读展示；凭据未配置或厂商 API 失败时降级为
// 只读台账（不发 5xx，保证「先有账号后配凭据」的渐进开通）。不读 secret 明文。
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

	account, err := s.db.GetCloudAccount(id)
	if errors.Is(err, storage.ErrNotFound) {
		s.handleError(w, r, http.StatusNotFound, "Cloud account not found")
		return
	}
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load cloud account")
		return
	}

	if provider, perr := s.cloudProvider(&account); perr == nil {
		if serr := s.syncCloudInstances(r.Context(), &account, provider); serr != nil {
			// 厂商 API 失败降级：仍返回已落库台账，不把发现失败当 5xx。
		}
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
			Region: req.Region, Scopes: req.Scopes, Enabled: *req.Enabled,
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
