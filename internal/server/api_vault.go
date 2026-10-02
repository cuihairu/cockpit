package server

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 远控凭据保险箱（阿里云 Workbench 密码箱模式，2026-10-02）：
// 凭据 AES-GCM 加密落库（storage.RemoteCredential），明文仅在连接注入时
// （handleTicketCreate 的 use_saved 路径）解密，从不回传浏览器。管理类操作
// （保存/删除）需二次验证——重输登录密码或 TOTP 动态码，通过后签发短期
// 多次使用的 vault token；使用已存凭据连接本身仅凭登录态（便利点所在）。

// vaultTokenTTL vault token 有效期：10 分钟内连续管理不用反复重验
const vaultTokenTTL = 10 * time.Minute

// vaultTokenEntry 一枚已签发的 vault token
type vaultTokenEntry struct {
	userID    string
	expiresAt time.Time
}

// vaultTokens 包级 token 存储（同 terminalSessions/guacRelayReg 惯例：
// 不依赖 Server 字段，测试可用零值 Server 走真实分发路径）。过期条目在
// 签发/校验时懒清理，不起常驻协程。
var (
	vaultTokensMu sync.Mutex
	vaultTokens   = map[string]*vaultTokenEntry{}
)

// vaultIssueToken 二次验证通过后签发 vault token
func vaultIssueToken(userID string) string {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		// randRead 实际不会失败（注入点仅供测试），兜底用时间熵保命
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	token := hex.EncodeToString(b)

	vaultTokensMu.Lock()
	now := time.Now()
	for t, e := range vaultTokens {
		if now.After(e.expiresAt) {
			delete(vaultTokens, t)
		}
	}
	vaultTokens[token] = &vaultTokenEntry{userID: userID, expiresAt: now.Add(vaultTokenTTL)}
	vaultTokensMu.Unlock()
	return token
}

// vaultCheckToken 校验 vault token 是否有效且属于该用户
func vaultCheckToken(token, userID string) bool {
	if token == "" {
		return false
	}
	vaultTokensMu.Lock()
	defer vaultTokensMu.Unlock()
	e, ok := vaultTokens[token]
	if !ok || time.Now().After(e.expiresAt) || e.userID != userID {
		return false
	}
	return true
}

// registerVaultAPI 注册保险箱路由。GET 列表只要登录态（弹窗打开时要用它
// 探测「已存凭据」，这正是仅凭登录态的连接便利）；PUT/DELETE 是管理操作，
// 需 X-Vault-Token（二次验证产物）。
func (s *Server) registerVaultAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/vault/verify", s.authService().Middleware(s.handleVaultVerify))
	mux.HandleFunc("/api/remote/vault/credentials", s.authService().Middleware(s.handleVaultCredentials))
	mux.HandleFunc("/api/remote/vault/credentials/", s.authService().Middleware(s.handleVaultCredentialDelete))
}

// handleVaultVerify 二次验证：重输登录密码或 TOTP 动态码 → 签发 vault token
func (s *Server) handleVaultVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password,omitempty"`
		TOTPCode string `json:"totp_code,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var verified bool
	var method string
	switch {
	case req.Password != "":
		// 登录密码重验（bcrypt 比对，VerifyPassword 内部按用户名取哈希）
		_, err := s.db.VerifyPassword(userInfo.Username, req.Password)
		verified = err == nil
		method = "password"
	case req.TOTPCode != "":
		valid, _, err := s.db.ValidateTOTPCode(userInfo.UserID, req.TOTPCode)
		verified = err == nil && valid
		method = "totp"
	default:
		http.Error(w, "password or totp_code required", http.StatusBadRequest)
		return
	}

	ip := s.getClientIP(r)
	ua := r.UserAgent()
	if !verified {
		s.auditVaultAction(audit.ActionVaultVerify, userInfo.UserID, userInfo.Username,
			audit.StatusFailure, ip, ua, "", map[string]string{"method": method, "reason": "verification failed"})
		http.Error(w, "Verification failed", http.StatusUnauthorized)
		return
	}

	token := vaultIssueToken(userInfo.UserID)
	s.auditVaultAction(audit.ActionVaultVerify, userInfo.UserID, userInfo.Username,
		audit.StatusSuccess, ip, ua, "", map[string]string{"method": method})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"expires_at": time.Now().Add(vaultTokenTTL).Format(time.RFC3339),
	})
}

// handleVaultCredentials GET 列表（元数据）/ PUT 保存（需 vault token）
func (s *Server) handleVaultCredentials(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		list, err := s.db.ListRemoteCredentials(userInfo.UserID)
		if err != nil {
			http.Error(w, "Failed to list credentials", http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": list})

	case http.MethodPut:
		if !vaultCheckToken(r.Header.Get("X-Vault-Token"), userInfo.UserID) {
			http.Error(w, "Vault verification required", http.StatusUnauthorized)
			return
		}
		s.handleVaultCredentialUpsert(w, r, userInfo)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleVaultCredentialUpsert(w http.ResponseWriter, r *http.Request, userInfo auth.UserInfo) {
	var req struct {
		AgentID    string `json:"agent_id"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Protocol   string `json:"protocol"`
		Username   string `json:"username,omitempty"`
		Domain     string `json:"domain,omitempty"`
		Password   string `json:"password,omitempty"`
		PrivateKey string `json:"private_key,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AgentID == "" || req.Host == "" || req.Port <= 0 || req.Protocol == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}
	switch req.Protocol {
	case "ssh", "rdp", "vnc":
	default:
		http.Error(w, "Invalid protocol", http.StatusBadRequest)
		return
	}
	if req.Password == "" && req.PrivateKey == "" {
		http.Error(w, "password or private_key required", http.StatusBadRequest)
		return
	}

	if err := s.db.UpsertRemoteCredential(&storage.RemoteCredentialInput{
		UserID:     userInfo.UserID,
		AgentID:    req.AgentID,
		Host:       req.Host,
		Port:       req.Port,
		Protocol:   req.Protocol,
		Username:   req.Username,
		Domain:     req.Domain,
		Password:   req.Password,
		PrivateKey: req.PrivateKey,
	}); err != nil {
		http.Error(w, "Failed to save credential", http.StatusInternalServerError)
		return
	}

	s.auditVaultAction(audit.ActionUpdate, userInfo.UserID, userInfo.Username,
		audit.StatusSuccess, s.getClientIP(r), r.UserAgent(),
		req.Protocol+"@"+req.Host, map[string]string{"agent_id": req.AgentID, "host": req.Host, "port": strconv.Itoa(req.Port), "protocol": req.Protocol})

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleVaultCredentialDelete DELETE /api/remote/vault/credentials/{id}（需 vault token）
func (s *Server) handleVaultCredentialDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !vaultCheckToken(r.Header.Get("X-Vault-Token"), userInfo.UserID) {
		http.Error(w, "Vault verification required", http.StatusUnauthorized)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/remote/vault/credentials/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "Missing credential id", http.StatusBadRequest)
		return
	}

	deleted, err := s.db.DeleteRemoteCredential(userInfo.UserID, id)
	if err != nil {
		http.Error(w, "Failed to delete credential", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "Credential not found", http.StatusNotFound)
		return
	}

	s.auditVaultAction(audit.ActionDelete, userInfo.UserID, userInfo.Username,
		audit.StatusSuccess, s.getClientIP(r), r.UserAgent(), id, nil)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// auditVaultAction 保险箱操作审计（details 只进资源标识，永不进凭据明文）
func (s *Server) auditVaultAction(action, userID, username, status, ip, userAgent, resourceID string, details interface{}) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Log(&audit.LogEntry{
		UserID:     userID,
		Username:   username,
		Action:     action,
		Resource:   audit.ResourceRemoteCredential,
		ResourceID: resourceID,
		Status:     status,
		IP:         ip,
		UserAgent:  userAgent,
		Details:    details,
	})
}
