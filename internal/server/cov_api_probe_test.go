package server

// api_probe.go 的覆盖率补充测试：registerProbeAPI、handleProbeAPI/handleNotificationAPI
// 分发、状态视图边界分支、落库失败、审计 username 分支。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
)

// ============ registerProbeAPI ============

func TestCovRegisterProbeAPI(t *testing.T) {
	s := newProbeTestServer(t)
	mux := http.NewServeMux()
	s.registerProbeAPI(mux)

	// 无认证 → 401
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/probe/config", nil))
	covWantCode(t, "probe no auth", rec, http.StatusUnauthorized)

	rec = covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/notification/status", nil))
	covWantCode(t, "notification no auth", rec, http.StatusUnauthorized)

	// 带认证 → 分发到对应 handler（token 由该 server 的 authService 签发）
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/probe/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "probe with auth", rec, http.StatusOK)

	req = covReq(http.MethodGet, "/api/notification/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "notification with auth", rec, http.StatusOK)
}

// ============ handleProbeAPI 分发 ============

func TestCovProbeAPIDispatch(t *testing.T) {
	s := newProbeTestServer(t)

	// config GET
	rec := covRec()
	s.handleProbeAPI(rec, covReq(http.MethodGet, "/api/probe/config", nil))
	covWantCode(t, "config get", rec, http.StatusOK)

	// config PUT（全量合法 body）
	rec = covRec()
	s.handleProbeAPI(rec, covReq(http.MethodPut, "/api/probe/config", strings.NewReader(probeConfigJSON(60, 2, 80, 85, 7, 30))))
	covWantCode(t, "config put", rec, http.StatusOK)

	// config 其他方法 → 405
	rec = covRec()
	s.handleProbeAPI(rec, covReq(http.MethodPost, "/api/probe/config", nil))
	covWantCode(t, "config 405", rec, http.StatusMethodNotAllowed)

	// history GET → 200（空结果）
	rec = covRec()
	s.handleProbeAPI(rec, covReq(http.MethodGet, "/api/probe/history?resource_type=service&resource_id=s1", nil))
	covWantCode(t, "history get", rec, http.StatusOK)

	// history 非 GET → 405
	rec = covRec()
	s.handleProbeAPI(rec, covReq(http.MethodPost, "/api/probe/history", nil))
	covWantCode(t, "history 405", rec, http.StatusMethodNotAllowed)

	// 未知路径 → 404
	rec = covRec()
	s.handleProbeAPI(rec, covReq(http.MethodGet, "/api/probe/whatever", nil))
	covWantCode(t, "unknown path", rec, http.StatusNotFound)
}

// ============ handleNotificationAPI 分发 ============

func TestCovNotificationAPIDispatch(t *testing.T) {
	s := newProbeTestServer(t)

	// status GET → 200
	rec := covRec()
	s.handleNotificationAPI(rec, covReq(http.MethodGet, "/api/notification/status", nil))
	covWantCode(t, "status get", rec, http.StatusOK)

	// status 非 GET → 405
	rec = covRec()
	s.handleNotificationAPI(rec, covReq(http.MethodPost, "/api/notification/status", nil))
	covWantCode(t, "status 405", rec, http.StatusMethodNotAllowed)

	// test POST → 未启用 notifier → 503（仍进入 test 分支）
	rec = covRec()
	s.handleNotificationAPI(rec, covReq(http.MethodPost, "/api/notification/test", nil))
	covWantCode(t, "test post disabled", rec, http.StatusServiceUnavailable)

	// test 非 POST → 405
	rec = covRec()
	s.handleNotificationAPI(rec, covReq(http.MethodGet, "/api/notification/test", nil))
	covWantCode(t, "test 405", rec, http.StatusMethodNotAllowed)

	// 未知路径 → 404
	rec = covRec()
	s.handleNotificationAPI(rec, covReq(http.MethodGet, "/api/notification/whatever", nil))
	covWantCode(t, "unknown path", rec, http.StatusNotFound)
}

// ============ handleNotificationStatus 边界分支 ============

func TestCovNotificationStatusBranches(t *testing.T) {
	db := testServerDB(t)

	// notifier nil + cfg nil：enabled=false，channels/events 为空数组
	s := &Server{db: db, audit: audit.NewLogger(db)}
	rec := covRec()
	s.handleNotificationStatus(rec, covReq(http.MethodGet, "/api/notification/status", nil))
	covWantCode(t, "notifier nil cfg nil", rec, http.StatusOK)

	// notifier 非 nil，cfg 非 nil 但 Notification 为 nil
	s2 := &Server{db: db, audit: audit.NewLogger(db), notifier: notification.NewService(nil), cfg: &config.Config{}}
	rec = covRec()
	s2.handleNotificationStatus(rec, covReq(http.MethodGet, "/api/notification/status", nil))
	covWantCode(t, "nil notification cfg", rec, http.StatusOK)

	// Events 含 nil 项：跳过，只输出合法事件
	cfg := &config.NotificationConfig{
		Enabled: true,
		Events: map[string]*config.EventConfig{
			"ok":  {Type: notification.ServiceDown, Enabled: true},
			"nil": nil,
		},
	}
	s3 := &Server{db: db, audit: audit.NewLogger(db), notifier: notification.NewService(cfg), cfg: &config.Config{Notification: cfg}}
	rec = covRec()
	s3.handleNotificationStatus(rec, covReq(http.MethodGet, "/api/notification/status", nil))
	covWantCode(t, "nil event entry", rec, http.StatusOK)
	var resp struct {
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Events) != 1 || resp.Events[0].Type != notification.ServiceDown {
		t.Errorf("events = %+v, want 1 valid entry", resp.Events)
	}
}

// ============ handleNotificationTest 边界分支 ============

func TestCovNotificationTestNilNotifier(t *testing.T) {
	s := &Server{} // notifier nil → 503
	rec := covRec()
	s.handleNotificationTest(rec, covReq(http.MethodPost, "/api/notification/test", nil))
	covWantCode(t, "nil notifier", rec, http.StatusServiceUnavailable)
}

// 失败渠道统计 + 审计 username 分支（webhook 指向本机未监听端口，立即连接拒绝）
func TestCovNotificationTestFailedChannelAndAudit(t *testing.T) {
	db := testServerDB(t)
	cfg := &config.NotificationConfig{
		Enabled: true,
		Webhook: []*config.WebhookConfig{{URL: "http://127.0.0.1:1/hook"}},
	}
	s := &Server{db: db, audit: audit.NewLogger(db), notifier: notification.NewService(cfg)}

	req := covAuthReq(http.MethodPost, "/api/notification/test", nil, "1", "admin", "admin")
	rec := covCallAuth(s, s.handleNotificationTest, req)
	covWantCode(t, "failed channel", rec, http.StatusOK)

	var resp struct {
		Results []struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].OK {
		t.Errorf("results = %+v, want 1 failed channel", resp.Results)
	}
}

// ============ alertThresholds：从 Setting 表读取 ============

func TestCovAlertThresholdsFromSettings(t *testing.T) {
	s := newProbeTestServer(t)
	settings := map[string]string{
		alert.DiskThresholdSettingKey:   "60",
		alert.MemoryThresholdSettingKey: "70",
		alert.CertWarnDaysSettingKey:    "5",
		alert.CertInfoDaysSettingKey:    "20",
	}
	for k, v := range settings {
		if err := s.db.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	disk, mem, warn, info := s.alertThresholds()
	if disk != 60 || mem != 70 || warn != 5 || info != 20 {
		t.Fatalf("thresholds = %d/%d/%d/%d, want 60/70/5/20", disk, mem, warn, info)
	}
}

// ============ saveProbeConfig / handleProbeHistory 错误分支 ============

func TestCovProbeConfigPutDBError(t *testing.T) {
	s := newProbeTestServer(t)
	covCloseDB(t, s)

	rec := covRec()
	s.handleProbeConfigPut(rec, covReq(http.MethodPut, "/api/probe/config", strings.NewReader(probeConfigJSON(60, 2, 80, 85, 7, 30))))
	covWantCode(t, "save setting error", rec, http.StatusInternalServerError)
}

// 带用户上下文保存 → auditProbeConfig 取到 username
func TestCovProbeConfigPutAuditsUsername(t *testing.T) {
	s := newProbeTestServer(t)

	req := covAuthReq(http.MethodPut, "/api/probe/config",
		strings.NewReader(probeConfigJSON(90, 3, 75, 80, 4, 25)), "1", "admin", "admin")
	rec := covCallAuth(s, s.handleProbeConfigPut, req)
	covWantCode(t, "put with user ctx", rec, http.StatusOK)
}

func TestCovProbeHistoryDBError(t *testing.T) {
	s := newProbeTestServer(t)
	covCloseDB(t, s)

	rec := covRec()
	s.handleProbeHistory(rec, covReq(http.MethodGet, "/api/probe/history?resource_type=service&resource_id=s1", nil))
	covWantCode(t, "history db error", rec, http.StatusInternalServerError)
}
