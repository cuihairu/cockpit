package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/probe"
	"github.com/cuihairu/cockpit/internal/storage"
)

func newProbeTestServer(t *testing.T) *Server {
	t.Helper()
	db := testServerDB(t)
	return &Server{
		db:          db,
		audit:       audit.NewLogger(db),
		probeRunner: probe.NewRunner(db, 0, nil),
		notifier:    notification.NewService(nil),
	}
}

// probeConfigJSON 全量 PUT body（M2/D14：config 是全量对象）
func probeConfigJSON(interval, fail, disk, mem, warn, info int) string {
	return fmt.Sprintf(`{"interval_seconds": %d, "fail_threshold": %d, "disk_percent": %d, "memory_percent": %d, "cert_warn_days": %d, "cert_info_days": %d}`,
		interval, fail, disk, mem, warn, info)
}

func TestProbeConfigGet(t *testing.T) {
	s := newProbeTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/probe/config", nil)
	rec := httptest.NewRecorder()
	s.handleProbeConfigGet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var resp probeConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.IntervalSeconds != 300 {
		t.Errorf("interval_seconds = %d, want 300 (default)", resp.IntervalSeconds)
	}
	if resp.MinIntervalSecond != 30 || resp.MaxIntervalSecond != 3600 {
		t.Errorf("bounds = [%d, %d], want [30, 3600]", resp.MinIntervalSecond, resp.MaxIntervalSecond)
	}
	// M2 阈值默认值
	if resp.FailThreshold != 2 || resp.DiskPercent != 80 || resp.MemoryPercent != 85 ||
		resp.CertWarnDays != 7 || resp.CertInfoDays != 30 {
		t.Errorf("thresholds = %+v, want defaults (2/80/85/7/30)", resp)
	}
}

func TestProbeConfigPutPersistsAndApplies(t *testing.T) {
	s := newProbeTestServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/probe/config",
		strings.NewReader(probeConfigJSON(60, 3, 90, 95, 3, 21)))
	rec := httptest.NewRecorder()
	s.handleProbeConfigPut(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := s.probeRunner.Interval(); got != time.Minute {
		t.Errorf("runner interval = %v, want 1m", got)
	}
	if got := s.probeRunner.FailThreshold(); got != 3 {
		t.Errorf("runner fail_threshold = %d, want 3", got)
	}

	// 落库：新 runner 以 DB 值启动（复用 startProbeRunner 的读取逻辑语义）
	v, err := s.db.GetSetting(probe.IntervalSettingKey)
	if err != nil || v != "60" {
		t.Errorf("stored setting = %q, err = %v, want 60", v, err)
	}
	if v, _ := s.db.GetSetting(alert.MemoryThresholdSettingKey); v != "95" {
		t.Errorf("stored memory threshold = %q, want 95", v)
	}

	// alert.Generator 下一轮读取生效
	g := alert.NewGenerator(s.db, nil, nil)
	g.CheckAllChecks() // loadThresholds 刷新
	// 通过导出行为验证：阈值快照直接读库比对即可
	if v, _ := s.db.GetSetting(alert.CertWarnDaysSettingKey); v != "3" {
		t.Errorf("stored cert warn days = %q, want 3", v)
	}
}

func TestProbeConfigPutValidatesRange(t *testing.T) {
	s := newProbeTestServer(t)
	bad := []string{
		`{"interval_seconds": 5}`,
		`{"interval_seconds": 7200}`,
		`{"bogus": 1}`,
		probeConfigJSON(60, 0, 80, 85, 7, 30),  // fail_threshold 下限
		probeConfigJSON(60, 11, 80, 85, 7, 30), // fail_threshold 上限
		probeConfigJSON(60, 2, 10, 85, 7, 30),  // disk 下限
		probeConfigJSON(60, 2, 80, 100, 7, 30), // mem 上限
		probeConfigJSON(60, 2, 80, 85, 0, 30),  // cert_warn 下限
		probeConfigJSON(60, 2, 80, 85, 7, 400), // cert_info 上限
		probeConfigJSON(60, 2, 80, 85, 30, 7),  // info < warn
	}
	for _, body := range bad {
		req := httptest.NewRequest(http.MethodPut, "/api/probe/config", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleProbeConfigPut(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s code = %d, want 400", body, rec.Code)
		}
	}
	// 非法请求不影响 runner
	if got := s.probeRunner.Interval(); got != 5*time.Minute {
		t.Errorf("runner interval changed by invalid request: %v", got)
	}
}

func TestProbeHistory(t *testing.T) {
	s := newProbeTestServer(t)
	rows := []*storage.ProbeResult{
		{ResourceType: "service", ResourceID: "svc-1", Name: "api", Status: "up", CheckedAt: time.Now().Add(-2 * time.Minute)},
		{ResourceType: "service", ResourceID: "svc-1", Name: "api", Status: "down", CheckedAt: time.Now()},
		{ResourceType: "service", ResourceID: "svc-2", Name: "db", Status: "up", CheckedAt: time.Now()},
	}
	if err := s.db.CreateProbeResults(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 正常查询：按目标过滤 + 倒序
	req := httptest.NewRequest(http.MethodGet, "/api/probe/history?resource_type=service&resource_id=svc-1", nil)
	rec := httptest.NewRecorder()
	s.handleProbeHistory(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []*storage.ProbeResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 2 || resp.Results[0].Status != "down" {
		t.Fatalf("results = %+v, want 2 rows newest first", resp.Results)
	}

	// limit 生效
	req = httptest.NewRequest(http.MethodGet, "/api/probe/history?resource_type=service&resource_id=svc-1&limit=1", nil)
	rec = httptest.NewRecorder()
	s.handleProbeHistory(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Results) != 1 {
		t.Errorf("limit=1 got %d rows, want 1", len(resp.Results))
	}

	// 缺参 / 非法类型 / 非法 limit
	for _, bad := range []string{
		"/api/probe/history",
		"/api/probe/history?resource_type=service",
		"/api/probe/history?resource_id=svc-1",
		"/api/probe/history?resource_type=agent&resource_id=x",
		"/api/probe/history?resource_type=service&resource_id=svc-1&limit=0",
		"/api/probe/history?resource_type=service&resource_id=svc-1&limit=999",
	} {
		req := httptest.NewRequest(http.MethodGet, bad, nil)
		rec := httptest.NewRecorder()
		s.handleProbeHistory(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s code = %d, want 400", bad, rec.Code)
		}
	}

	// 空结果返回空数组而非 null
	s2 := newProbeTestServer(t)
	req = httptest.NewRequest(http.MethodGet, "/api/probe/history?resource_type=domain&resource_id=none", nil)
	rec = httptest.NewRecorder()
	s2.handleProbeHistory(rec, req)
	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Errorf("empty history should marshal as [], got %s", rec.Body.String())
	}
}

func TestProbeConfigRunnerNotStarted(t *testing.T) {
	s := newProbeTestServer(t)
	s.probeRunner = nil

	req := httptest.NewRequest(http.MethodGet, "/api/probe/config", nil)
	rec := httptest.NewRecorder()
	s.handleProbeConfigGet(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("get code = %d, want 503", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/probe/config", strings.NewReader(`{"interval_seconds":60}`))
	rec = httptest.NewRecorder()
	s.handleProbeConfigPut(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("put code = %d, want 503", rec.Code)
	}
}

func TestNotificationStatusNoSecrets(t *testing.T) {
	db := testServerDB(t)
	capURL := "http://127.0.0.1:1"
	cfg := &config.NotificationConfig{
		Enabled: true,
		Herald:  &config.HeraldConfig{BaseURL: capURL},
		Webhook: []*config.WebhookConfig{{URL: capURL + "/hook", Secret: "topsecret"}},
		Events: map[string]*config.EventConfig{
			"service-down": {Type: notification.ServiceDown, Enabled: true},
			"service-up":   {Type: notification.ServiceUp, Enabled: false},
		},
	}
	s := &Server{db: db, cfg: &config.Config{Notification: cfg}, notifier: notification.NewService(cfg)}

	req := httptest.NewRequest(http.MethodGet, "/api/notification/status", nil)
	rec := httptest.NewRecorder()
	s.handleNotificationStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"enabled":true`) {
		t.Errorf("status should report enabled, body: %s", body)
	}
	if strings.Contains(body, "topsecret") {
		t.Error("status leaks webhook secret")
	}
	var resp notificationStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Errorf("channels = %+v, want 2", resp.Channels)
	}
	if len(resp.Events) != 2 {
		t.Errorf("events = %+v, want 2", resp.Events)
	}
}

func TestNotificationTestRequiresEnabledService(t *testing.T) {
	s := newProbeTestServer(t) // notifier = NewService(nil) → disabled
	req := httptest.NewRequest(http.MethodPost, "/api/notification/test", nil)
	rec := httptest.NewRecorder()
	s.handleNotificationTest(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestNotificationTestDispatches(t *testing.T) {
	db := testServerDB(t)
	// dispatch 并发扇出到各渠道，hits 必须原子（非原子 ++ 在多核下丢增，
	// 曾致 CI 偶发 hits=1）
	var hits atomic.Int64
	capSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer capSrv.Close()

	cfg := &config.NotificationConfig{
		Enabled: true,
		Herald:  &config.HeraldConfig{BaseURL: capSrv.URL},
		Webhook: []*config.WebhookConfig{{URL: capSrv.URL + "/hook"}},
		Events:  map[string]*config.EventConfig{}, // 空白名单：test 仍应投递
	}
	s := &Server{db: db, audit: audit.NewLogger(db), notifier: notification.NewService(cfg)}

	req := httptest.NewRequest(http.MethodPost, "/api/notification/test", nil)
	rec := httptest.NewRecorder()
	s.handleNotificationTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []notification.SendResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %+v, want 2", resp.Results)
	}
	for _, r := range resp.Results {
		if !r.OK {
			t.Errorf("channel %s failed: %s", r.Channel, r.Error)
		}
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2 (herald + webhook)", hits.Load())
	}
}
