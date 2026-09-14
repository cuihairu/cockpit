package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/probe"
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

func TestProbeConfigGet(t *testing.T) {
	s := newProbeTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/probe/config", nil)
	rec := httptest.NewRecorder()
	s.handleProbeConfigGet(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var resp struct {
		IntervalSeconds   int64 `json:"interval_seconds"`
		MinIntervalSecond int64 `json:"min_interval_seconds"`
		MaxIntervalSecond int64 `json:"max_interval_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.IntervalSeconds != 300 {
		t.Errorf("interval_seconds = %d, want 300 (default)", resp.IntervalSeconds)
	}
	if resp.MinIntervalSecond != 30 || resp.MaxIntervalSecond != 3600 {
		t.Errorf("bounds = [%d, %d], want [30, 3600]", resp.MinIntervalSecond, resp.MaxIntervalSecond)
	}
}

func TestProbeConfigPutPersistsAndApplies(t *testing.T) {
	s := newProbeTestServer(t)

	body := `{"interval_seconds": 60}`
	req := httptest.NewRequest(http.MethodPut, "/api/probe/config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleProbeConfigPut(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := s.probeRunner.Interval(); got != time.Minute {
		t.Errorf("runner interval = %v, want 1m", got)
	}

	// 落库：新 runner 以 DB 值启动（复用 startProbeRunner 的读取逻辑语义）
	v, err := s.db.GetSetting(probe.IntervalSettingKey)
	if err != nil || v != "60" {
		t.Errorf("stored setting = %q, err = %v, want 60", v, err)
	}
}

func TestProbeConfigPutValidatesRange(t *testing.T) {
	s := newProbeTestServer(t)
	for _, bad := range []string{`{"interval_seconds": 5}`, `{"interval_seconds": 7200}`, `{"bogus": 1}`} {
		req := httptest.NewRequest(http.MethodPut, "/api/probe/config", strings.NewReader(bad))
		rec := httptest.NewRecorder()
		s.handleProbeConfigPut(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s code = %d, want 400", bad, rec.Code)
		}
	}
	// 非法请求不影响 runner
	if got := s.probeRunner.Interval(); got != 5*time.Minute {
		t.Errorf("runner interval changed by invalid request: %v", got)
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
	var hits int
	capSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
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
	if hits != 2 {
		t.Errorf("hits = %d, want 2 (herald + webhook)", hits)
	}
}
