package notification

// cov_channels_test.go 覆盖各通知渠道与 Service 的错误/边界分支：
// herald 未配置、ntfy 优先级与请求错误、telegram 本地可测的错误路径、
// webhook 错误路径、Client.SendEvent 的各前置校验，以及 Service 的 nil 接收者分支。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

func TestCovHeraldChannelNotConfigured(t *testing.T) {
	h := &heraldChannel{client: nil, target: "http://x"}
	err := h.Send(context.Background(), &Notification{EventType: ServiceDown})
	if err == nil || !strings.Contains(err.Error(), "herald channel not configured") {
		t.Fatalf("err = %v, want not configured", err)
	}
	if h.Name() != "herald" {
		t.Errorf("Name = %s", h.Name())
	}
}

func TestCovNtfyPriority(t *testing.T) {
	explicit := newNtfyChannel(&config.NtfyConfig{Server: "http://x", Topic: "t", Priority: "urgent"})
	if got := explicit.priority(&Notification{Level: "error"}); got != "urgent" {
		t.Errorf("explicit priority = %s, want urgent", got)
	}
	derived := newNtfyChannel(&config.NtfyConfig{Server: "http://x", Topic: "t"})
	if got := derived.priority(&Notification{Level: "warning"}); got != "default" {
		t.Errorf("warning priority = %s, want default", got)
	}
}

func TestCovNtfySendCreateRequestError(t *testing.T) {
	c := newNtfyChannel(&config.NtfyConfig{Server: "http://bad\x7fhost", Topic: "t"})
	if err := c.Send(context.Background(), &Notification{Title: "x"}); err == nil {
		t.Fatal("invalid server URL should fail request creation")
	}
}

func TestCovNtfySendSuccessWithTokenAndError(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ntfy 渠道会把 server URL 规范成末带 / 的路径
		if strings.TrimSuffix(r.URL.Path, "/") == "/ok" {
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	ok := newNtfyChannel(&config.NtfyConfig{Server: srv.URL + "/ok", Topic: "t", Token: "tok-cov"})
	if err := ok.Send(context.Background(), &Notification{Title: "x", Level: "info"}); err != nil {
		t.Fatalf("send error = %v", err)
	}
	if gotAuth != "Bearer tok-cov" {
		t.Errorf("auth = %q", gotAuth)
	}

	bad := newNtfyChannel(&config.NtfyConfig{Server: srv.URL + "/err", Topic: "t"})
	err := bad.Send(context.Background(), &Notification{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "unexpected status code 500") {
		t.Fatalf("err = %v, want 500", err)
	}
}

func TestCovTelegramSendCreateRequestError(t *testing.T) {
	// token 含控制字符 → URL 构造失败，无需真实网络
	c := newTelegramChannel(&config.TelegramConfig{BotToken: "bad\ntoken", ChatID: "1"})
	if err := c.Send(context.Background(), &Notification{Title: "x"}); err == nil {
		t.Fatal("invalid bot token should fail request creation")
	}
}

func TestCovTelegramSendContextCanceled(t *testing.T) {
	// 已取消的 ctx 让 client.Do 立刻失败，不触达真实 Telegram API
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTelegramChannel(&config.TelegramConfig{BotToken: "123:abc", ChatID: "42"})
	err := c.Send(ctx, &Notification{Title: "x", Message: "m", ResourceType: "service", ResourceID: "s1"})
	if err == nil {
		t.Fatal("canceled context should fail send")
	}
}

func TestCovWebhookSendErrors(t *testing.T) {
	badURL := newWebhookChannel(&config.WebhookConfig{URL: "http://bad\x7fhost/hook"})
	if err := badURL.Send(context.Background(), &Notification{EventType: ServiceDown}); err == nil {
		t.Fatal("invalid URL should fail request creation")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	c := newWebhookChannel(&config.WebhookConfig{URL: srv.URL, Secret: "s"})
	err := c.Send(context.Background(), &Notification{EventType: ServiceDown})
	if err == nil || !strings.Contains(err.Error(), "unexpected status code 502") {
		t.Fatalf("err = %v, want 502", err)
	}
}

func TestCovSendEventGuardClauses(t *testing.T) {
	// nil client
	var nilClient *Client
	if err := nilClient.SendEvent(context.Background(), &Event{Type: ServiceDown}); err == nil {
		t.Fatal("nil client should error")
	}
	// config 存在但 Herald 未配置
	c := NewClient(&config.NotificationConfig{})
	if err := c.SendEvent(context.Background(), &Event{Type: ServiceDown}); err == nil {
		t.Fatal("missing herald config should error")
	}
	// event 为 nil：静默返回 nil
	cfg := &config.NotificationConfig{Herald: &config.HeraldConfig{BaseURL: "http://127.0.0.1:1"}}
	c2 := NewClient(cfg)
	if err := c2.SendEvent(context.Background(), nil); err != nil {
		t.Fatalf("nil event should be no-op, got %v", err)
	}
	// BaseURL 为空
	cfg2 := &config.NotificationConfig{Herald: &config.HeraldConfig{BaseURL: ""}}
	if err := NewClient(cfg2).SendEvent(context.Background(), &Event{Type: ServiceDown}); err == nil {
		t.Fatal("empty base URL should error")
	}
}

func TestCovSendEventCreateRequestError(t *testing.T) {
	cfg := &config.NotificationConfig{Herald: &config.HeraldConfig{BaseURL: "http://bad\x7fherald"}}
	if err := NewClient(cfg).SendEvent(context.Background(), &Event{Type: ServiceDown}); err == nil {
		t.Fatal("invalid herald URL should fail request creation")
	}
}

func TestCovSendEventNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	cfg := &config.NotificationConfig{Herald: &config.HeraldConfig{BaseURL: srv.URL}}
	if err := NewClient(cfg).SendEvent(context.Background(), &Event{Type: ServiceDown}); err == nil {
		t.Fatal("dead server should error")
	}
}

func TestCovSendAlertNilClient(t *testing.T) {
	var nilClient *Client
	if err := nilClient.SendAlert(context.Background(), &storage.Alert{}); err == nil {
		t.Fatal("nil client should error")
	}
}

func TestCovAlertToEventDisabled(t *testing.T) {
	cfg := &config.NotificationConfig{
		Enabled: true,
		Events: map[string]*config.EventConfig{
			"sd": {Type: ServiceDown, Enabled: false},
		},
	}
	rt := "service"
	if ev := AlertToEvent(&storage.Alert{Type: "error", Title: "服务宕机", ResourceType: &rt}, cfg); ev != nil {
		t.Errorf("disabled event should yield nil, got %+v", ev)
	}
}

func TestCovServiceNilReceiverBranches(t *testing.T) {
	var s *Service
	if got := s.ChannelSummaries(); got != nil {
		t.Errorf("nil service summaries = %v", got)
	}
	if got := s.SendAll(context.Background(), &Notification{EventType: ServiceDown}); got != nil {
		t.Errorf("nil service SendAll = %v", got)
	}
	if got := s.TestAll(context.Background()); got != nil {
		t.Errorf("nil service TestAll = %v", got)
	}
	s.SendNonBlocking(&Notification{EventType: ServiceDown})     // nil service：直接返回
	s.SendAlertNonBlocking(&storage.Alert{Type: "error"})        // nil service：直接返回
	s2 := NewService(nil)
	s2.SendNonBlocking(nil)                                      // nil 通知：直接返回
	s2.SendAlertNonBlocking(nil)                                 // nil 告警：直接返回
}

func TestCovSendNonBlockingFilteredEvent(t *testing.T) {
	s := NewService(testServiceConfig("http://127.0.0.1:1"))
	// ServiceUp 未在白名单：不投递
	s.SendNonBlocking(&Notification{EventType: ServiceUp, Title: "t"})
	// AlertToNotification nil 告警
	if n := AlertToNotification(nil); n != nil {
		t.Errorf("nil alert should yield nil notification")
	}
}

func TestCovSendNonBlockingLogsFailures(t *testing.T) {
	// 全部渠道快速失败（本地不可达端口），异步 dispatch 走失败日志分支
	cfg := &config.NotificationConfig{
		Enabled: true,
		Herald:  &config.HeraldConfig{BaseURL: "http://127.0.0.1:1"},
		Ntfy:    []*config.NtfyConfig{{Server: "http://127.0.0.1:1", Topic: "t"}},
		Webhook: []*config.WebhookConfig{{URL: "http://127.0.0.1:1/hook"}},
		Events:  map[string]*config.EventConfig{"sd": {Type: ServiceDown, Enabled: true}},
	}
	s := NewService(cfg)
	s.SendNonBlocking(&Notification{EventType: ServiceDown, Title: "t"})
	// dispatch 在后台 goroutine，等待其完成（连接拒绝是即时的）
	time.Sleep(300 * time.Millisecond)
}

func TestCovTargetHostTruncation(t *testing.T) {
	long := strings.Repeat("a", 60)
	if got := targetHost(long); got != long[:40] {
		t.Errorf("targetHost(long) = %q, want 40-char prefix", got)
	}
	if got := targetHost(strings.Repeat("a", 20)); got != strings.Repeat("a", 20) {
		t.Errorf("short input should return as-is")
	}
}
