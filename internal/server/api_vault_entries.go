package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/vault"
)

// 通用密码箱条目管理 API（vault-design.md V2/V6/V7）：vault.provider 所选
// provider 的读写删面。管理操作（写/删）与远控凭据共用同一 vault token 门
// （登录口令/TOTP 重验签发，10 分钟）；列表只回元数据，明文永不出库。

// vaultEntryPathRe 条目 path 约束：<域>/<名>，域与名用小写字母数字与分隔符
var vaultEntryPathRe = regexp.MustCompile(`^[a-z0-9][a-z0-9/_.-]*$`)

// vaultEntryFromPath 校验并截取 URL 中的条目 path。
func vaultEntryFromPath(raw string) (string, bool) {
	p := strings.TrimPrefix(raw, "/api/remote/vault/entries/")
	p = strings.Trim(p, "/")
	if p == "" || strings.Contains(p, "..") || !vaultEntryPathRe.MatchString(p) {
		return "", false
	}
	return p, true
}

// handleVaultEntry PUT 写入 / DELETE 删除单条（需 vault token）
func (s *Server) handleVaultEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if s.vaultProvider == nil {
		http.Error(w, "Vault unavailable", http.StatusServiceUnavailable)
		return
	}
	entryPath, ok := vaultEntryFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid entry path", http.StatusBadRequest)
		return
	}
	if !vaultCheckToken(r.Header.Get("X-Vault-Token"), userInfo.UserID) {
		http.Error(w, "Vault verification required", http.StatusUnauthorized)
		return
	}

	ip := s.getClientIP(r)
	ua := r.UserAgent()
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Kind     string            `json:"kind,omitempty"`
			Username string            `json:"username,omitempty"`
			Value    string            `json:"value"`
			Meta     map[string]string `json:"meta,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Kind == "" {
			req.Kind = "pair"
		}
		if req.Value == "" && req.Username == "" {
			http.Error(w, "value or username required", http.StatusBadRequest)
			return
		}
		err := s.vaultProvider.Write(entryPath, &vault.Entry{
			Path:     entryPath,
			Kind:     req.Kind,
			Username: req.Username,
			Value:    req.Value,
			Meta:     req.Meta,
		})
		if err != nil {
			s.auditVaultAction(audit.ActionUpdate, userInfo.UserID, userInfo.Username,
				audit.StatusFailure, ip, ua, entryPath, map[string]string{"reason": "write failed"})
			http.Error(w, "Failed to write entry", http.StatusInternalServerError)
			return
		}
		s.auditVaultAction(audit.ActionUpdate, userInfo.UserID, userInfo.Username,
			audit.StatusSuccess, ip, ua, entryPath, map[string]string{"kind": req.Kind})
		// NAS 目标变更即对在线 agent 补推（vault-design.md V6）
		if strings.HasPrefix(entryPath, nasVaultPrefix) {
			go s.pushNasTargetsAll()
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "path": entryPath})

	case http.MethodDelete:
		err := s.vaultProvider.Delete(entryPath)
		if err == vault.ErrNotFound {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		if err != nil {
			s.auditVaultAction(audit.ActionDelete, userInfo.UserID, userInfo.Username,
				audit.StatusFailure, ip, ua, entryPath, map[string]string{"reason": "delete failed"})
			http.Error(w, "Failed to delete entry", http.StatusInternalServerError)
			return
		}
		s.auditVaultAction(audit.ActionDelete, userInfo.UserID, userInfo.Username,
			audit.StatusSuccess, ip, ua, entryPath, nil)
		if strings.HasPrefix(entryPath, nasVaultPrefix) {
			go s.pushNasTargetsAll()
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}
}

// handleVaultEntries GET 列表（?prefix=，元数据形态）
func (s *Server) handleVaultEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userInfo, ok := auth.GetUserFromContext(r)
	if !ok || userInfo.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if s.vaultProvider == nil {
		http.Error(w, "Vault unavailable", http.StatusServiceUnavailable)
		return
	}
	prefix := r.URL.Query().Get("prefix")
	metas, err := s.vaultProvider.List(prefix)
	if err != nil {
		http.Error(w, "Failed to list entries", http.StatusInternalServerError)
		return
	}
	s.auditVaultAction(audit.ActionView, userInfo.UserID, userInfo.Username,
		audit.StatusSuccess, s.getClientIP(r), r.UserAgent(), prefix, nil)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": metas})
}
