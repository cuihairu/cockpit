package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// captureServer 记录收到的请求（path + 解码后的 JSON body）
type captureServer struct {
	srv    *httptest.Server
	paths  chan string
	bodies chan map[string]interface{}
	closed atomic.Int32
}

func newCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	c := &captureServer{
		paths:  make(chan string, 8),
		bodies: make(chan map[string]interface{}, 8),
	}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		c.paths <- r.URL.Path
		c.bodies <- payload
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

// testServiceConfig 构建四渠道全部指向本地可测地址的配置
// （telegram 硬编码官方 API，无法本地化，由调用方决定是否纳入断言）
func testServiceConfig(heraldURL string) *config.NotificationConfig {
	return &config.NotificationConfig{
		Enabled: true,
		Herald: &config.HeraldConfig{
			BaseURL: heraldURL,
			Timeout: time.Second,
		},
		Ntfy: []*config.NtfyConfig{
			{Server: "https://ntfy.example.com", Topic: "cockpit-alerts"},
		},
		Webhook: []*config.WebhookConfig{
			{URL: "https://hooks.example.com/cockpit", Secret: "s3cret"},
		},
		Telegram: []*config.TelegramConfig{
			{BotToken: "123:abc", ChatID: "42"},
		},
		Events: map[string]*config.EventConfig{
			"service-down": {Type: ServiceDown, Enabled: true},
		},
	}
}

func TestNewServiceDisabled(t *testing.T) {
	if s := NewService(nil); s.Enabled() {
		t.Error("nil config service should be disabled")
	}
	if s := NewService(&config.NotificationConfig{}); s.Enabled() {
		t.Error("disabled config service should be disabled")
	}
}

func TestServiceChannelSummariesNoSecrets(t *testing.T) {
	s := NewService(testServiceConfig("http://127.0.0.1:1"))
	summaries := s.ChannelSummaries()
	if len(summaries) != 4 {
		t.Fatalf("expected 4 channels, got %d", len(summaries))
	}
	names := map[string]string{}
	for _, c := range summaries {
		names[c.Channel] = c.Target
	}
	if names["herald"] != "127.0.0.1:1" {
		t.Errorf("herald target = %q", names["herald"])
	}
	if names["ntfy"] != "cockpit-alerts" {
		t.Errorf("ntfy target = %q", names["ntfy"])
	}
	if names["webhook"] != "hooks.example.com" {
		t.Errorf("webhook target = %q (should be host only)", names["webhook"])
	}
	if names["telegram"] != "42" {
		t.Errorf("telegram target = %q", names["telegram"])
	}
	for _, c := range summaries {
		for _, secret := range []string{"s3cret", "123:abc"} {
			if c.Target == secret {
				t.Errorf("target %q leaks credential", c.Target)
			}
		}
	}
}

func TestServiceIsEventEnabledWhitelist(t *testing.T) {
	s := NewService(testServiceConfig("http://127.0.0.1:1"))
	if !s.IsEventEnabled(ServiceDown) {
		t.Error("explicitly enabled event should pass")
	}
	if s.IsEventEnabled(ServiceUp) {
		t.Error("unconfigured event should be filtered (whitelist)")
	}
	if s.IsEventEnabled(EventTest) {
		t.Error("test event is not part of whitelist")
	}
	var nilSvc *Service
	if nilSvc.IsEventEnabled(ServiceDown) {
		t.Error("nil service should filter everything")
	}
}

func TestServiceSendAllDispatchesToAllChannels(t *testing.T) {
	herald := newCaptureServer(t)
	webhook := newCaptureServer(t)
	ntfy := newCaptureServer(t)

	cfg := testServiceConfig(herald.srv.URL)
	cfg.Webhook[0].URL = webhook.srv.URL + "/hook"
	cfg.Ntfy[0].Server = ntfy.srv.URL

	s := NewService(cfg)
	n := &Notification{
		EventType:    ServiceDown,
		Title:        "服务宕机",
		Message:      "web down",
		Level:        "error",
		ResourceType: "service",
		ResourceID:   "svc-1",
		Time:         time.Now(),
	}
	results := s.SendAll(context.Background(), n)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d: %+v", len(results), results)
	}
	byChannel := map[string]SendResult{}
	for _, r := range results {
		byChannel[r.Channel] = r
	}
	// telegram 指向官方 API，本地环境失败可接受，但结果必须存在
	if r, ok := byChannel["telegram"]; ok && r.OK {
		t.Log("telegram unexpectedly succeeded")
	}
	for _, name := range []string{"herald", "webhook", "ntfy"} {
		if r := byChannel[name]; !r.OK {
			t.Errorf("channel %s failed: %s", name, r.Error)
		}
	}

	// herald：{type, labels} 与历史行为一致
	select {
	case payload := <-herald.bodies:
		if payload["type"] != ServiceDown {
			t.Errorf("herald type = %v", payload["type"])
		}
		labels, _ := payload["labels"].(map[string]interface{})
		if labels["title"] != "服务宕机" || labels["resource_id"] != "svc-1" {
			t.Errorf("herald labels = %+v", labels)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no herald request")
	}

	// webhook：完整载荷 + secret 头
	select {
	case payload := <-webhook.bodies:
		if payload["event_type"] != ServiceDown || payload["title"] != "服务宕机" {
			t.Errorf("webhook payload = %+v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no webhook request")
	}

	// ntfy：JSON 发布模式带 topic
	select {
	case payload := <-ntfy.bodies:
		if payload["topic"] != "cockpit-alerts" {
			t.Errorf("ntfy topic = %v", payload["topic"])
		}
		if payload["priority"] != "high" {
			t.Errorf("ntfy priority = %v (error level → high)", payload["priority"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no ntfy request")
	}
}

func TestServiceSendAllFiltersDisabledEvents(t *testing.T) {
	s := NewService(testServiceConfig("http://127.0.0.1:1"))
	results := s.SendAll(context.Background(), &Notification{EventType: ServiceUp})
	if results != nil {
		t.Errorf("filtered event should return nil, got %+v", results)
	}
}

func TestServiceSendAllChannelFailureIsolated(t *testing.T) {
	herald := newCaptureServer(t)
	webhook := newCaptureServer(t)
	cfg := testServiceConfig(herald.srv.URL)
	cfg.Webhook[0].URL = webhook.srv.URL + "/hook"
	// ntfy 指向不可达端口模拟单渠道故障
	cfg.Ntfy[0].Server = "http://127.0.0.1:1"

	s := NewService(cfg)
	results := s.SendAll(context.Background(), &Notification{EventType: ServiceDown, Title: "t"})
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	byChannel := map[string]SendResult{}
	for _, r := range results {
		byChannel[r.Channel] = r
	}
	if !byChannel["webhook"].OK {
		t.Errorf("webhook should succeed despite ntfy failure: %s", byChannel["webhook"].Error)
	}
	if !byChannel["herald"].OK {
		t.Errorf("herald should succeed: %s", byChannel["herald"].Error)
	}
	if byChannel["ntfy"].OK {
		t.Error("ntfy should fail (unreachable)")
	}
}

func TestServiceTestAllBypassesWhitelist(t *testing.T) {
	herald := newCaptureServer(t)
	cfg := testServiceConfig(herald.srv.URL)
	// events 白名单里没有 test 事件
	s := NewService(cfg)
	results := s.TestAll(context.Background())
	if len(results) != 4 {
		t.Fatalf("test notification should dispatch to all 4 channels, got %d", len(results))
	}
	byChannel := map[string]SendResult{}
	for _, r := range results {
		byChannel[r.Channel] = r
	}
	if !byChannel["herald"].OK {
		t.Errorf("herald should receive test event: %s", byChannel["herald"].Error)
	}
	select {
	case payload := <-herald.bodies:
		if payload["type"] != EventTest {
			t.Errorf("test event type = %v", payload["type"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no herald request")
	}
}

func TestServiceSendAlertNonBlocking(t *testing.T) {
	capSrv := newCaptureServer(t)
	cfg := testServiceConfig(capSrv.srv.URL)
	cfg.Webhook[0].URL = capSrv.srv.URL + "/hook"
	cfg.Ntfy[0].Server = capSrv.srv.URL
	s := NewService(cfg)

	resourceType := "service"
	resourceID := "svc-9"
	alert := &storage.Alert{
		Type:         "error",
		Title:        "服务宕机",
		Message:      "服务 api 处于宕机状态",
		ResourceID:   &resourceID,
		ResourceType: &resourceType,
	}
	s.SendAlertNonBlocking(alert)

	deadline := time.After(3 * time.Second)
	var sawWebhookPayload bool
	for i := 0; i < 3; i++ { // herald / webhook / ntfy 三个本地渠道
		select {
		case payload := <-capSrv.bodies:
			if payload["event_type"] == ServiceDown && payload["title"] == "服务宕机" {
				sawWebhookPayload = true
			}
		case <-deadline:
			t.Fatalf("timed out; sawWebhookPayload=%v", sawWebhookPayload)
		}
	}
	if !sawWebhookPayload {
		t.Error("expected webhook payload with event_type=service.down")
	}

	// 不可推导事件类型的告警不投递：仅清空 channel 无新请求
	emptyCap := newCaptureServer(t)
	cfg2 := testServiceConfig(emptyCap.srv.URL)
	s2 := NewService(cfg2)
	s2.SendAlertNonBlocking(&storage.Alert{Type: "info", Title: "普通消息"})
	select {
	case payload := <-emptyCap.bodies:
		t.Errorf("unmatched alert should not be delivered, got %+v", payload)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestTargetHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://ntfy.example.com", "ntfy.example.com"},
		{"http://127.0.0.1:8080/base", "127.0.0.1:8080"},
		{"not-a-url", "not-a-url"},
	}
	for _, c := range cases {
		if got := targetHost(c.in); got != c.want {
			t.Errorf("targetHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
