package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/probe"
)

// 拨测与通知 API（拨测增强，设计见 docs/guide/probe-enhance-design.md）。
//
//	GET  /api/probe/config          当前探测间隔
//	PUT  /api/probe/config          修改探测间隔（落库 + 立即生效 + 审计）
//	GET  /api/notification/status   通知服务状态（渠道摘要 + 事件开关，不含凭据）
//	POST /api/notification/test     向全部渠道发送测试通知（审计）
const (
	probeAPIPrefix        = "/api/probe/"
	notificationAPIPrefix = "/api/notification/"
)

func (s *Server) registerProbeAPI(mux *http.ServeMux) {
	mux.HandleFunc(probeAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleProbeAPI)(w, r)
	})
	mux.HandleFunc(notificationAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleNotificationAPI)(w, r)
	})
}

func (s *Server) handleProbeAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/probe/config" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleProbeConfigGet(w, r)
	case http.MethodPut:
		s.handleProbeConfigPut(w, r)
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// probeConfigResponse 探测配置视图
type probeConfigResponse struct {
	IntervalSeconds   int64 `json:"interval_seconds"`
	MinIntervalSecond int64 `json:"min_interval_seconds"`
	MaxIntervalSecond int64 `json:"max_interval_seconds"`
}

func (s *Server) handleProbeConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.probeRunner == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "probe runner not started")
		return
	}
	s.writeJSON(w, http.StatusOK, probeConfigResponse{
		IntervalSeconds:   int64(s.probeRunner.Interval() / time.Second),
		MinIntervalSecond: probe.MinIntervalSeconds,
		MaxIntervalSecond: probe.MaxIntervalSeconds,
	})
}

func (s *Server) handleProbeConfigPut(w http.ResponseWriter, r *http.Request) {
	if s.probeRunner == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "probe runner not started")
		return
	}

	var req struct {
		IntervalSeconds int64 `json:"interval_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.IntervalSeconds < probe.MinIntervalSeconds || req.IntervalSeconds > probe.MaxIntervalSeconds {
		s.handleError(w, r, http.StatusBadRequest, "interval_seconds out of range [30, 3600]")
		return
	}

	interval := time.Duration(req.IntervalSeconds) * time.Second
	s.probeRunner.SetInterval(interval)
	if err := s.db.SetSetting(probe.IntervalSettingKey, strconv.FormatInt(req.IntervalSeconds, 10)); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to save probe config")
		return
	}

	s.auditProbeConfig(r, req.IntervalSeconds)
	log.Printf("[probe] interval updated to %s by API", interval)
	s.writeJSON(w, http.StatusOK, probeConfigResponse{
		IntervalSeconds:   req.IntervalSeconds,
		MinIntervalSecond: probe.MinIntervalSeconds,
		MaxIntervalSecond: probe.MaxIntervalSeconds,
	})
}

func (s *Server) auditProbeConfig(r *http.Request, intervalSeconds int64) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, audit.ActionUpdate, audit.ResourceProbe, probe.IntervalSettingKey,
		map[string]interface{}{"interval_seconds": intervalSeconds},
		s.getClientIP(r), r.UserAgent())
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
