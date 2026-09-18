package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ACME 证书签发 API（设计见 docs/guide/acme-design.md D9/D10/D12）。
//
//	GET    /api/acme/certs                      列表（元数据，PEM 不出响应）
//	POST   /api/acme/certs                      创建（审计 acme_create）
//	PUT    /api/acme/certs/{id}                 更新（审计 acme_update）
//	DELETE /api/acme/certs/{id}                 删除（审计 acme_delete；不吊销）
//	POST   /api/acme/certs/{id}/issue           立即签发/重签（审计 acme_issue）
//	GET    /api/acme/certs/{id}/download?part=  下载 PEM（part=key 强制记审计）
//	GET    /api/acme/account                    账户信息（email/注册状态）
//	PUT    /api/acme/account                    设置 email（审计 acme_update）
//	GET/PUT /api/acme/config                    全局续期巡检间隔（0 = 关闭）

// handleACME 分发 /api/acme[...]
func (s *Server) handleACME(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/acme"), "/")
	switch {
	case sub == "config":
		s.handleAcmeConfigAPI(w, r)
	case sub == "account":
		s.handleAcmeAccountAPI(w, r)
	case sub == "certs":
		switch r.Method {
		case http.MethodGet:
			s.handleAcmeList(w, r)
		case http.MethodPost:
			s.handleAcmeCreate(w, r)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasPrefix(sub, "certs/"):
		rest := strings.TrimPrefix(sub, "certs/") // "{id}" / "{id}/issue" / "{id}/download"
		s.handleAcmeCertSub(w, r, rest)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleAcmeCertSub 分发 /api/acme/certs/{id}[/{action}]
func (s *Server) handleAcmeCertSub(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case strings.HasSuffix(rest, "/issue"):
		idStr := strings.TrimSuffix(rest, "/issue")
		s.handleAcmeIssue(w, r, idStr)
	case strings.HasSuffix(rest, "/download"):
		idStr := strings.TrimSuffix(rest, "/download")
		s.handleAcmeDownload(w, r, idStr)
	default:
		id, err := strconv.Atoi(rest)
		if err != nil || id <= 0 {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		switch r.Method {
		case http.MethodPut:
			s.handleAcmeUpdate(w, r, uint(id))
		case http.MethodDelete:
			s.handleAcmeDelete(w, r, uint(id))
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// acmeCertInput 创建/更新请求体
type acmeCertInput struct {
	Domains         []string `json:"domains"`
	CADirectory     string   `json:"caDirectory"`
	AutoRenew       bool     `json:"autoRenew"`
	RenewBeforeDays *int     `json:"renewBeforeDays"` // 指针：缺省（nil）用默认 30，显式 0 视为非法
}

var acmeDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*\.)+[a-zA-Z]{2,}$`)

// validateAcmeDomains 校验域名列表（D12）：非空；泛域名剥 `*.` 前缀后
// 过 DNS 记录名同款正则（前端同规则双端校验）。返回归一化列表
// （trim、去 `*.` 前缀内的空格）。
func validateAcmeDomains(domains []string) ([]string, error) {
	if len(domains) == 0 {
		return nil, errString("at least one domain is required")
	}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		name := strings.TrimPrefix(d, "*.")
		if !acmeDomainRe.MatchString(name) {
			return nil, errString("invalid domain: " + d)
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, errString("at least one domain is required")
	}
	return out, nil
}

// validateAcmeInput 完整校验，返回（归一化 domains, 归一化 CA 目录, renewBeforeDays, 错误）
func validateAcmeInput(in acmeCertInput) ([]string, string, int, error) {
	domains, err := validateAcmeDomains(in.Domains)
	if err != nil {
		return nil, "", 0, err
	}
	ca := strings.ToLower(strings.TrimSpace(in.CADirectory))
	if ca == "" {
		ca = AcmeCADirectoryStaging // 默认 staging 防误触生产限频（D4）
	}
	if ca != AcmeCADirectoryStaging && ca != AcmeCADirectoryProduction {
		return nil, "", 0, errString("caDirectory must be staging or production")
	}
	days := 30
	if in.RenewBeforeDays != nil {
		days = *in.RenewBeforeDays
	}
	if days < 7 || days > 90 {
		return nil, "", 0, errString("renewBeforeDays must be in [7, 90]")
	}
	return domains, ca, days, nil
}

// acmeCertView 响应视图（白名单字段；三段 PEM 绝不出响应，D9）
type acmeCertView struct {
	ID              uint      `json:"id"`
	Domains         []string  `json:"domains"`
	PrimaryDomain   string    `json:"primaryDomain"`
	CADirectory     string    `json:"caDirectory"`
	Status          string    `json:"status"`
	HasPrivateKey   bool      `json:"hasPrivateKey"`
	ExpiresAt       time.Time `json:"expiresAt"`
	RenewBeforeDays int       `json:"renewBeforeDays"`
	AutoRenew       bool      `json:"autoRenew"`
	LastRenewAt     int64     `json:"lastRenewAt"`
	LastStatus      string    `json:"lastStatus"`
	LastError       string    `json:"lastError"`
	CheckedAt       int64     `json:"checkedAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func toAcmeCertView(c *storage.AcmeCert) acmeCertView {
	return acmeCertView{
		ID:              c.ID,
		Domains:         c.Domains,
		PrimaryDomain:   c.PrimaryDomain,
		CADirectory:     c.CADirectory,
		Status:          c.Status,
		HasPrivateKey:   c.PrivateKeyPEM != "",
		ExpiresAt:       c.ExpiresAt,
		RenewBeforeDays: c.RenewBeforeDays,
		AutoRenew:       c.AutoRenew,
		LastRenewAt:     c.LastRenewAt,
		LastStatus:      c.LastStatus,
		LastError:       c.LastError,
		CheckedAt:       c.CheckedAt,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

// auditAcme 记审计（resourceType=acme_cert，resourceID=主域名/{id}）
func (s *Server) auditAcme(r *http.Request, action, resourceID string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceAcmeCert,
		resourceID, details, s.getClientIP(r), r.UserAgent())
}

func (s *Server) handleAcmeList(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListAcmeCerts()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list ACME certificates")
		return
	}
	views := make([]acmeCertView, 0, len(list))
	for _, c := range list {
		views = append(views, toAcmeCertView(c))
	}
	s.writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleAcmeCreate(w http.ResponseWriter, r *http.Request) {
	var in acmeCertInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	domains, ca, days, err := validateAcmeInput(in)
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	cert := &storage.AcmeCert{
		Domains:         domains,
		PrimaryDomain:   domains[0],
		CADirectory:     ca,
		Status:          "pending",
		RenewBeforeDays: days,
		AutoRenew:       in.AutoRenew,
		LastStatus:      "never",
	}
	if err := s.db.CreateAcmeCert(cert); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create ACME certificate config")
		return
	}
	s.auditAcme(r, "acme_create", cert.PrimaryDomain, in)
	s.writeJSON(w, http.StatusOK, toAcmeCertView(cert))
}

func (s *Server) handleAcmeUpdate(w http.ResponseWriter, r *http.Request, id uint) {
	cert, err := s.db.GetAcmeCert(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "ACME certificate not found")
		return
	}
	var in acmeCertInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	domains, ca, days, err := validateAcmeInput(in)
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// 域名/CA 变更后旧产物不再对应，重置为待签发
	if !equalDomains(cert.Domains, domains) || cert.CADirectory != ca {
		cert.Status = "pending"
		cert.CertificatePEM, cert.IssuerPEM, cert.PrivateKeyPEM = "", "", ""
		cert.ExpiresAt = time.Time{}
		cert.LastStatus = "never"
		cert.LastError = ""
	}
	cert.Domains = domains
	cert.PrimaryDomain = domains[0]
	cert.CADirectory = ca
	cert.RenewBeforeDays = days
	cert.AutoRenew = in.AutoRenew
	if err := s.db.UpdateAcmeCert(cert); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to update ACME certificate config")
		return
	}
	s.auditAcme(r, "acme_update", cert.PrimaryDomain, in)
	s.writeJSON(w, http.StatusOK, toAcmeCertView(cert))
}

func equalDomains(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Server) handleAcmeDelete(w http.ResponseWriter, r *http.Request, id uint) {
	cert, err := s.db.GetAcmeCert(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "ACME certificate not found")
		return
	}
	// 仅删本地记录，不向 CA 吊销（D9）
	if err := s.db.DeleteAcmeCert(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to delete ACME certificate")
		return
	}
	s.auditAcme(r, "acme_delete", cert.PrimaryDomain, map[string]string{"caDirectory": cert.CADirectory})
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleAcmeIssue 立即签发/重签（同步）：token 未配置 503（D12），
// 其余签发失败原样 400 返回 lego 错误（前端展示传播超时等细节）
func (s *Server) handleAcmeIssue(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	cert, err := s.db.GetAcmeCert(uint(id))
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "ACME certificate not found")
		return
	}
	if s.acme == nil || s.cfg == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "ACME issuer not configured")
		return
	}
	// DNS 凭据就绪判定按 ACME provider 分派（D13，与 DNS 管理页的
	// Cloudflare 判定语义分叉），错误文案报缺哪个键
	if ok, msg := s.acmeDNS().Ready(); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, msg)
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	if _, err := s.runACMEIssue(cert, generator, time.Now()); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.auditAcme(r, "acme_issue", cert.PrimaryDomain, map[string]interface{}{
		"caDirectory": cert.CADirectory,
		"domains":     cert.Domains,
	})
	s.writeJSON(w, http.StatusOK, toAcmeCertView(cert))
}

// handleAcmeDownload 下载 PEM（D9）：part=cert|issuer|key；
// key 强制记审计（私钥出库），cert/issuer 不记。
func (s *Server) handleAcmeDownload(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	cert, err := s.db.GetAcmeCert(uint(id))
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "ACME certificate not found")
		return
	}
	part := r.URL.Query().Get("part")
	if part == "" {
		part = "cert"
	}
	var body, filename string
	switch part {
	case "cert":
		body, filename = cert.CertificatePEM, cert.PrimaryDomain+".crt.pem"
	case "issuer":
		body, filename = cert.IssuerPEM, cert.PrimaryDomain+".issuer.pem"
	case "key":
		body, filename = cert.PrivateKeyPEM, cert.PrimaryDomain+".key.pem"
		s.auditAcme(r, "acme_download_key", cert.PrimaryDomain, map[string]string{"part": "key"})
	default:
		s.handleError(w, r, http.StatusBadRequest, "part must be cert, issuer or key")
		return
	}
	if body == "" {
		s.handleError(w, r, http.StatusNotFound, "certificate not issued yet")
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(body))
}

// handleAcmeAccountAPI 账户：GET 返回注册状态与 email（私钥不出）；
// PUT 设置 email（审计 acme_update；生效于下次签发时的注册/更新）
func (s *Server) handleAcmeAccountAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		acc, err := s.db.GetAcmeAccount()
		if err != nil {
			// 未注册返回空账户视图（不是错误——引导用户设置 email）
			s.writeJSON(w, http.StatusOK, map[string]interface{}{"registered": false})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"registered":      true,
			"email":           acc.Email,
			"caDirectory":     acc.CADirectory,
			"registrationURI": acc.RegistrationURI,
		})
	case http.MethodPut:
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		email := strings.TrimSpace(req.Email)
		if email != "" && !acmeEmailRe.MatchString(email) {
			s.handleError(w, r, http.StatusBadRequest, "invalid email")
			return
		}
		if _, err := s.db.GetAcmeAccount(); err != nil {
			if err := s.db.SaveAcmeAccount(&storage.AcmeAccount{Email: email}); err != nil {
				s.handleError(w, r, http.StatusInternalServerError, "failed to save ACME account")
				return
			}
		} else if err := s.db.UpdateAcmeAccountEmail(email); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "failed to update ACME account")
			return
		}
		s.auditAcme(r, "acme_update", "account", map[string]string{"email": email})
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

var acmeEmailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// handleAcmeConfigAPI 全局续期巡检配置：GET 返回当前间隔与范围；
// PUT 校验后写入 Setting（0 = 关闭）——与 /api/ddns/config 同构
func (s *Server) handleAcmeConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// dns 字段是 ACME 视角的 provider 与凭据就绪判定（D13，
		// 与 DNS 管理页 /api/dns/status 的 Cloudflare 专属语义分叉）
		dnsCfg := s.acmeDNS()
		provider := dnsCfg.Provider
		if provider == "" {
			provider = "cloudflare"
		}
		configured, _ := dnsCfg.Ready()
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": s.GetAcmeScanInterval(),
			"min":                   acmeMinIntervalSeconds,
			"max":                   acmeMaxIntervalSeconds,
			"default":               acmeDefaultInterval,
			"dns": map[string]interface{}{
				"provider":   provider,
				"configured": configured,
			},
		})
	case http.MethodPut:
		var req struct {
			ScanIntervalSeconds int `json:"scan_interval_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := s.SetAcmeScanInterval(req.ScanIntervalSeconds); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"scan_interval_seconds": req.ScanIntervalSeconds,
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}
