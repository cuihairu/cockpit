package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/probe"
)

// 拨测与通知 API（拨测增强，设计见 docs/guide/probe-enhance-design.md）。
//
//	GET  /api/probe/config          探测间隔 + 失败阈值 + 告警阈值（M2 扩容）
//	PUT  /api/probe/config          全量保存（逐字段校验落库 + 立即生效 + 审计）
//	GET  /api/probe/history         拨测历史（按目标取最近 N 条，心跳条数据源）
//	GET  /api/notification/status   通知服务状态（渠道摘要 + 事件开关，不含凭据）
//	POST /api/notification/test     向全部渠道发送测试通知（审计）
const (
	probeAPIPrefix        = "/api/probe/"
	notificationAPIPrefix = "/api/notification/"
)

// probeHistoryMaxLimit 单次历史查询上限；probeHistoryTypes 可查询的目标类型
const (
	probeHistoryDefaultLimit = 50
	probeHistoryMaxLimit     = 200
)

var probeHistoryTypes = map[string]bool{"service": true, "domain": true, "certificate": true}

func (s *Server) registerProbeAPI(mux *http.ServeMux) {
	mux.HandleFunc(probeAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleProbeAPI)(w, r)
	})
	mux.HandleFunc(notificationAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleNotificationAPI)(w, r)
	})
}

func (s *Server) handleProbeAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/probe/config":
		switch r.Method {
		case http.MethodGet:
			s.handleProbeConfigGet(w, r)
		case http.MethodPut:
			s.handleProbeConfigPut(w, r)
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/api/probe/history":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleProbeHistory(w, r)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// probeConfigResponse 探测与告警阈值配置视图（M2/D14：interval 字段语义不变，
// 其余字段为 M2 新增）
type probeConfigResponse struct {
	IntervalSeconds   int64 `json:"interval_seconds"`
	FailThreshold     int   `json:"fail_threshold"`
	DiskPercent       int   `json:"disk_percent"`
	MemoryPercent     int   `json:"memory_percent"`
	CertWarnDays      int   `json:"cert_warn_days"`
	CertInfoDays      int   `json:"cert_info_days"`
	MinIntervalSecond int64 `json:"min_interval_seconds"`
	MaxIntervalSecond int64 `json:"max_interval_seconds"`
	MinFailThreshold  int   `json:"min_fail_threshold"`
	MaxFailThreshold  int   `json:"max_fail_threshold"`
	MinPercent        int   `json:"min_percent"`
	MaxPercent        int   `json:"max_percent"`
}

// alertThresholds 从 Setting 表读告警阈值（缺省回退默认值）；
// alert.Generator 每轮自行刷新，server 侧直读库即可，不依赖其实例
func (s *Server) alertThresholds() (disk, mem, warnDays, infoDays int) {
	disk, mem = alert.DefaultDiskThreshold, alert.DefaultMemoryThreshold
	warnDays, infoDays = alert.DefaultCertWarnDays, alert.DefaultCertInfoDays
	readInt := func(key string, dst *int) {
		if v, err := s.db.GetSetting(key); err == nil && v != "" {
			if n, convErr := strconv.Atoi(v); convErr == nil {
				*dst = n
			}
		}
	}
	readInt(alert.DiskThresholdSettingKey, &disk)
	readInt(alert.MemoryThresholdSettingKey, &mem)
	readInt(alert.CertWarnDaysSettingKey, &warnDays)
	readInt(alert.CertInfoDaysSettingKey, &infoDays)
	return
}

func (s *Server) handleProbeConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.probeRunner == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "probe runner not started")
		return
	}
	disk, mem, warnDays, infoDays := s.alertThresholds()
	s.writeJSON(w, http.StatusOK, probeConfigResponse{
		IntervalSeconds:   int64(s.probeRunner.Interval() / time.Second),
		FailThreshold:     s.probeRunner.FailThreshold(),
		DiskPercent:       disk,
		MemoryPercent:     mem,
		CertWarnDays:      warnDays,
		CertInfoDays:      infoDays,
		MinIntervalSecond: probe.MinIntervalSeconds,
		MaxIntervalSecond: probe.MaxIntervalSeconds,
		MinFailThreshold:  probe.MinFailThreshold,
		MaxFailThreshold:  probe.MaxFailThreshold,
		MinPercent:        alert.MinPercentThreshold,
		MaxPercent:        alert.MaxPercentThreshold,
	})
}

func (s *Server) handleProbeConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.probeRunner == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "probe runner not started")
		return
	}

	var req probeConfigResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	switch {
	case req.IntervalSeconds < probe.MinIntervalSeconds || req.IntervalSeconds > probe.MaxIntervalSeconds:
		s.handleError(w, r, http.StatusBadRequest, "interval_seconds out of range [30, 3600]")
	case req.FailThreshold < probe.MinFailThreshold || req.FailThreshold > probe.MaxFailThreshold:
		s.handleError(w, r, http.StatusBadRequest, "fail_threshold out of range [1, 10]")
	case req.DiskPercent < alert.MinPercentThreshold || req.DiskPercent > alert.MaxPercentThreshold:
		s.handleError(w, r, http.StatusBadRequest, "disk_percent out of range [50, 99]")
	case req.MemoryPercent < alert.MinPercentThreshold || req.MemoryPercent > alert.MaxPercentThreshold:
		s.handleError(w, r, http.StatusBadRequest, "memory_percent out of range [50, 99]")
	case req.CertWarnDays < alert.MinCertWarnDays || req.CertWarnDays > alert.MaxCertWarnDays:
		s.handleError(w, r, http.StatusBadRequest, "cert_warn_days out of range [1, 90]")
	case req.CertInfoDays < alert.MinCertInfoDays || req.CertInfoDays > alert.MaxCertInfoDays:
		s.handleError(w, r, http.StatusBadRequest, "cert_info_days out of range [1, 365]")
	case req.CertInfoDays < req.CertWarnDays:
		// info 档小于 warn 档会让 warn 档形成死区
		s.handleError(w, r, http.StatusBadRequest, "cert_info_days must be >= cert_warn_days")
	default:
		s.saveProbeConfig(w, r, &req)
	}
}

// saveProbeConfig 校验通过后的落库与生效（D14：一次审计）
func (s *Server) saveProbeConfig(w http.ResponseWriter, r *http.Request, req *probeConfigResponse) {
	interval := time.Duration(req.IntervalSeconds) * time.Second
	s.probeRunner.SetInterval(interval)
	s.probeRunner.SetFailThreshold(req.FailThreshold)

	settings := map[string]string{
		probe.IntervalSettingKey:        strconv.FormatInt(req.IntervalSeconds, 10),
		probe.FailThresholdSettingKey:   strconv.Itoa(req.FailThreshold),
		alert.DiskThresholdSettingKey:   strconv.Itoa(req.DiskPercent),
		alert.MemoryThresholdSettingKey: strconv.Itoa(req.MemoryPercent),
		alert.CertWarnDaysSettingKey:    strconv.Itoa(req.CertWarnDays),
		alert.CertInfoDaysSettingKey:    strconv.Itoa(req.CertInfoDays),
	}
	for key, value := range settings {
		if err := s.db.SetSetting(key, value); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Failed to save probe config")
			return
		}
	}

	s.auditProbeConfig(r, req)
	log.Printf("[probe] config updated by API (interval %s, fail_threshold %d, disk %d%%, mem %d%%, cert %d/%dd)",
		interval, req.FailThreshold, req.DiskPercent, req.MemoryPercent, req.CertWarnDays, req.CertInfoDays)
	s.writeJSON(w, http.StatusOK, req)
}

func (s *Server) auditProbeConfig(r *http.Request, req *probeConfigResponse) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, audit.ActionUpdate, audit.ResourceProbe, probe.IntervalSettingKey,
		map[string]interface{}{
			"interval_seconds": req.IntervalSeconds,
			"fail_threshold":   req.FailThreshold,
			"disk_percent":     req.DiskPercent,
			"memory_percent":   req.MemoryPercent,
			"cert_warn_days":   req.CertWarnDays,
			"cert_info_days":   req.CertInfoDays,
		},
		s.getClientIP(r), r.UserAgent())
}

// handleProbeHistory 拨测历史查询（D11）：resource_type + resource_id 必填，
// limit 默认 50 上限 200，checked_at 倒序
func (s *Server) handleProbeHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resourceType := q.Get("resource_type")
	resourceID := q.Get("resource_id")
	if !probeHistoryTypes[resourceType] || resourceID == "" {
		s.handleError(w, r, http.StatusBadRequest, "resource_type (service|domain|certificate) and resource_id are required")
		return
	}

	limit := probeHistoryDefaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > probeHistoryMaxLimit {
			s.handleError(w, r, http.StatusBadRequest, "limit must be in [1, 200]")
			return
		}
		limit = n
	}

	results, err := s.db.ListProbeResults(resourceType, resourceID, limit)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to load probe history")
		return
	}
	// gorm Find 空表恒返回非 nil 空 slice，无需兜底
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

func (s *Server) handleNotificationAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/notification/status":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleNotificationStatus(w, r)
	case "/api/notification/test":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleNotificationTest(w, r)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// notificationStatusResponse 通知服务状态视图（渠道只给摘要，不含凭据）
type notificationStatusResponse struct {
	Enabled  bool                   `json:"enabled"`
	Channels []notificationSummary  `json:"channels"`
	Events   []notificationEventRow `json:"events"`
}

type notificationSummary struct {
	Channel string `json:"channel"`
	Target  string `json:"target"`
}

type notificationEventRow struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleNotificationStatus(w http.ResponseWriter, r *http.Request) {
	resp := notificationStatusResponse{Channels: []notificationSummary{}, Events: []notificationEventRow{}}
	if s.notifier != nil {
		resp.Enabled = s.notifier.Enabled()
		for _, c := range s.notifier.ChannelSummaries() {
			resp.Channels = append(resp.Channels, notificationSummary{Channel: c.Channel, Target: c.Target})
		}
	}
	if s.cfg != nil && s.cfg.Notification != nil {
		for _, ec := range s.cfg.Notification.Events {
			if ec == nil {
				continue
			}
			resp.Events = append(resp.Events, notificationEventRow{Type: ec.Type, Enabled: ec.Enabled})
		}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	if s.notifier == nil || !s.notifier.Enabled() {
		s.handleError(w, r, http.StatusServiceUnavailable, "notification not enabled (check config.yaml notification section)")
		return
	}

	results := s.notifier.TestAll(r.Context())

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	sent, failed := 0, 0
	details := make([]map[string]interface{}, 0, len(results))
	for _, res := range results {
		if res.OK {
			sent++
		} else {
			failed++
		}
		details = append(details, map[string]interface{}{
			"channel": res.Channel, "target": res.Target, "ok": res.OK, "error": res.Error,
		})
	}
	s.audit.LogResource(username, "test", audit.ResourceNotification, "channels",
		map[string]interface{}{"sent": sent, "failed": failed, "results": details},
		s.getClientIP(r), r.UserAgent())

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}
