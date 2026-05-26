package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

func strPtr(s string) *string {
	return &s
}

func TestAlertToEvent(t *testing.T) {
	cfg := &config.NotificationConfig{
		Enabled: true,
		Events: map[string]*config.EventConfig{
			"certificate_expired": {
				Type:    "certificate.expired",
				Enabled: true,
			},
			"service_down": {
				Type:    "service.down",
				Enabled: true,
			},
		},
	}

	tests := []struct {
		name        string
		alert       *storage.Alert
		wantType    string
		wantEnabled bool
	}{
		{
			name: "无资源类型时返回nil",
			alert: &storage.Alert{
				Type:    "error",
				Title:   "证书已过期",
				Message: "域名 example.com 的证书已过期",
			},
			wantType:    "",
			wantEnabled: false,
		},
		{
			name: "证书过期事件",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "证书已过期",
				Message:      "域名 example.com 的证书已过期",
				ResourceType: strPtr("certificate"),
			},
			wantType:    "certificate.expired",
			wantEnabled: true,
		},
		{
			name: "服务宕机事件",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "服务宕机",
				Message:      "服务 nginx 宕机",
				ResourceType: strPtr("service"),
			},
			wantType:    "service.down",
			wantEnabled: true,
		},
		{
			name: "配置未启用时返回nil",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "证书已过期",
				Message:      "域名 example.com 的证书已过期",
				ResourceType: strPtr("certificate"),
			},
			wantType:    "",
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCfg := cfg
			if tt.name == "配置未启用时返回nil" {
				testCfg = &config.NotificationConfig{
					Enabled: false,
					Events: map[string]*config.EventConfig{
						"certificate_expired": {
							Type:    "certificate.expired",
							Enabled: true,
						},
					},
				}
			}

			event := AlertToEvent(tt.alert, testCfg)
			if tt.wantType == "" {
				if event != nil {
					t.Errorf("Expected nil event, got %+v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("Expected event, got nil")
			}
			if event.Type != tt.wantType {
				t.Errorf("Event.Type = %q, want %q", event.Type, tt.wantType)
			}
		})
	}
}

func TestGetAlertEventType(t *testing.T) {
	tests := []struct {
		name     string
		alert    *storage.Alert
		wantType string
	}{
		{
			name:     "无资源类型",
			alert:    &storage.Alert{Type: "error", Title: "证书已过期"},
			wantType: "",
		},
		{
			name: "证书已过期",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "证书已过期",
				ResourceType: strPtr("certificate"),
			},
			wantType: "certificate.expired",
		},
		{
			name: "证书7天过期",
			alert: &storage.Alert{
				Type:         "warning",
				Title:        "证书将在7天内过期",
				ResourceType: strPtr("certificate"),
			},
			wantType: "certificate.expiring",
		},
		{
			name: "证书30天过期",
			alert: &storage.Alert{
				Type:         "info",
				Title:        "证书将在30天内过期",
				ResourceType: strPtr("certificate"),
			},
			wantType: "certificate.warning",
		},
		{
			name: "服务宕机",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "服务nginx宕机",
				ResourceType: strPtr("service"),
			},
			wantType: "service.down",
		},
		{
			name: "Agent离线",
			alert: &storage.Alert{
				Type:         "warning",
				Title:        "Agent已离线",
				ResourceType: strPtr("agent"),
			},
			wantType: "agent.offline",
		},
		{
			name: "域名过期",
			alert: &storage.Alert{
				Type:         "error",
				Title:        "域名example.com已过期",
				ResourceType: strPtr("domain"),
			},
			wantType: "domain.expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAlertEventType(tt.alert)
			if got != tt.wantType {
				t.Errorf("getAlertEventType() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestBuildEventLabels(t *testing.T) {
	alert := &storage.Alert{
		Type:         "error",
		Title:        "证书已过期",
		Message:      "域名 example.com 的证书已过期",
		ResourceID:   strPtr("cert-123"),
		ResourceType: strPtr("certificate"),
	}

	labels := buildEventLabels(alert)

	expectedLabels := map[string]string{
		"level":         "error",
		"title":         "证书已过期",
		"message":       "域名 example.com 的证书已过期",
		"resource_id":   "cert-123",
		"resource_type": "certificate",
	}

	for k, v := range expectedLabels {
		if labels[k] != v {
			t.Errorf("Labels[%q] = %q, want %q", k, labels[k], v)
		}
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.NotificationConfig
		wantNil  bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantNil: true,
		},
		{
			name: "valid config",
			cfg: &config.NotificationConfig{
				Enabled: true,
				Herald: &config.HeraldConfig{
					BaseURL: "http://localhost:8080",
					Timeout: 5 * time.Second,
				},
			},
			wantNil: false,
		},
		{
			name: "config with default timeout",
			cfg: &config.NotificationConfig{
				Enabled: true,
				Herald: &config.HeraldConfig{
					BaseURL: "http://localhost:8080",
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			if (client == nil) != tt.wantNil {
				t.Errorf("NewClient() = %v, wantNil %v", client == nil, tt.wantNil)
			}
			if client != nil && tt.cfg != nil && tt.cfg.Herald != nil && tt.cfg.Herald.Timeout > 0 {
				if client.httpClient.Timeout != tt.cfg.Herald.Timeout {
					t.Errorf("client timeout = %v, want %v", client.httpClient.Timeout, tt.cfg.Herald.Timeout)
				}
			}
		})
	}
}

func TestClientIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.NotificationConfig
		expected bool
	}{
		{
			name:     "nil client",
			cfg:      nil,
			expected: false,
		},
		{
			name: "disabled config",
			cfg: &config.NotificationConfig{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled config",
			cfg: &config.NotificationConfig{
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			if client.IsEnabled() != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", client.IsEnabled(), tt.expected)
			}
		})
	}
}

func TestClientIsEventEnabled(t *testing.T) {
	cfg := &config.NotificationConfig{
		Enabled: true,
		Events: map[string]*config.EventConfig{
			"cert_expired": {
				Type:    "certificate.expired",
				Enabled: true,
			},
			"service_down": {
				Type:    "service.down",
				Enabled: false,
			},
		},
	}

	client := NewClient(cfg)

	if !client.IsEventEnabled("certificate.expired") {
		t.Error("Expected certificate.expired to be enabled")
	}

	if client.IsEventEnabled("service.down") {
		t.Error("Expected service.down to be disabled")
	}

	if client.IsEventEnabled("unknown.event") {
		t.Error("Expected unknown.event to be disabled")
	}

	// 测试禁用的客户端
	disabledClient := NewClient(&config.NotificationConfig{Enabled: false})
	if disabledClient.IsEventEnabled("certificate.expired") {
		t.Error("Expected all events to be disabled when client is disabled")
	}
}

func TestClientSendEvent(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectError    bool
		expectedStatus int
	}{
		{
			name: "successful send",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}
				if r.URL.Path != "/api/v1/events" {
					t.Errorf("expected path /api/v1/events, got %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusCreated)
			},
			expectError:    false,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal server error"))
			},
			expectError:    true,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "bad request",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("bad request"))
			},
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			cfg := &config.NotificationConfig{
				Enabled: true,
				Herald: &config.HeraldConfig{
					BaseURL: server.URL,
					Timeout: 5 * time.Second,
				},
			}

			client := NewClient(cfg)
			event := &Event{
				Type: "test.event",
				Labels: map[string]string{
					"key": "value",
				},
			}

			err := client.SendEvent(context.Background(), event)
			if (err != nil) != tt.expectError {
				t.Errorf("SendEvent() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestClientSendAlert(t *testing.T) {
	var receivedEvent map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/events" {
			t.Errorf("expected path /api/v1/events, got %s", r.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		receivedEvent = body

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := &config.NotificationConfig{
		Enabled: true,
		Herald: &config.HeraldConfig{
			BaseURL: server.URL,
			Timeout: 5 * time.Second,
		},
		Events: map[string]*config.EventConfig{
			"cert_expired": {
				Type:    "certificate.expired",
				Enabled: true,
			},
		},
	}

	client := NewClient(cfg)
	alert := &storage.Alert{
		Type:         "error",
		Title:        "证书已过期",
		Message:      "域名 example.com 的证书已过期",
		ResourceType: strPtr("certificate"),
		ResourceID:   strPtr("cert-123"),
	}

	err := client.SendAlert(context.Background(), alert)
	if err != nil {
		t.Fatalf("SendAlert() error = %v", err)
	}

	if receivedEvent == nil {
		t.Fatal("no event was sent")
	}

	if receivedEvent["type"] != "certificate.expired" {
		t.Errorf("event type = %v, want certificate.expired", receivedEvent["type"])
	}

	labels, ok := receivedEvent["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("labels is not a map")
	}

	if labels["title"] != "证书已过期" {
		t.Errorf("label title = %v, want 证书已过期", labels["title"])
	}
}

func TestClientSendAlertNoMatch(t *testing.T) {
	cfg := &config.NotificationConfig{
		Enabled: true,
		Herald: &config.HeraldConfig{
			BaseURL: "http://localhost:8080",
		},
		Events: map[string]*config.EventConfig{
			"cert_expired": {
				Type:    "certificate.expired",
				Enabled: true,
			},
		},
	}

	client := NewClient(cfg)
	alert := &storage.Alert{
		Type:         "error",
		Title:        "未知错误",
		Message:      "发生了一些错误",
		ResourceType: strPtr("unknown"),
	}

	err := client.SendAlert(context.Background(), alert)
	if err != nil {
		t.Fatalf("SendAlert() should not return error for non-matching alert, got %v", err)
	}
}

func TestSendAlertNonBlocking(t *testing.T) {
	tests := []struct {
		name           string
		client         *Client
		alert          *storage.Alert
		cfg            *config.NotificationConfig
		expectPanic    bool
		expectedStatus int
	}{
		{
			name:        "nil client does not panic",
			client:      nil,
			alert:       &storage.Alert{Type: "error", Title: "test"},
			cfg:         &config.NotificationConfig{},
			expectPanic: false,
		},
		{
			name: "disabled client does not send",
			client: NewClient(&config.NotificationConfig{
				Enabled: false,
			}),
			alert: &storage.Alert{Type: "error", Title: "test"},
			cfg:   &config.NotificationConfig{},
			expectPanic: false,
		},
		{
			name: "successful non-blocking send",
			client: func() *Client {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
				}))
				t.Cleanup(server.Close)

				return NewClient(&config.NotificationConfig{
					Enabled: true,
					Herald: &config.HeraldConfig{
						BaseURL: server.URL,
						Timeout: 5 * time.Second,
					},
					Events: map[string]*config.EventConfig{
						"cert_expired": {
							Type:    "certificate.expired",
							Enabled: true,
						},
					},
				})
			}(),
			alert: &storage.Alert{
				Type:         "error",
				Title:        "证书已过期",
				Message:      "域名 example.com 的证书已过期",
				ResourceType: strPtr("certificate"),
			},
			cfg:            &config.NotificationConfig{},
			expectPanic:    false,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "non-blocking returns immediately",
			client: func() *Client {
				// 创建一个会延迟响应的服务器
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusCreated)
				}))
				t.Cleanup(server.Close)

				return NewClient(&config.NotificationConfig{
					Enabled: true,
					Herald: &config.HeraldConfig{
						BaseURL: server.URL,
						Timeout: 5 * time.Second,
					},
					Events: map[string]*config.EventConfig{
						"cert_expired": {
							Type:    "certificate.expired",
							Enabled: true,
						},
					},
				})
			}(),
			alert: &storage.Alert{
				Type:         "error",
				Title:        "证书已过期",
				ResourceType: strPtr("certificate"),
			},
			cfg:         &config.NotificationConfig{},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试不会 panic
			defer func() {
				if r := recover(); r != nil && !tt.expectPanic {
					t.Errorf("SendAlertNonBlocking() panicked unexpectedly: %v", r)
				}
			}()

			start := time.Now()
			SendAlertNonBlocking(tt.client, tt.alert, tt.cfg)
			elapsed := time.Since(start)

			// 非阻塞调用应该几乎立即返回（< 10ms）
			if elapsed > 50*time.Millisecond {
				t.Errorf("SendAlertNonBlocking() took too long: %v, expected < 50ms", elapsed)
			}

			// 等待异步操作完成（仅用于验证，实际使用中不需要）
			time.Sleep(200 * time.Millisecond)
		})
	}
}

func TestSendAlertNonBlockingError(t *testing.T) {
	// 创建一个会返回错误的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewClient(&config.NotificationConfig{
		Enabled: true,
		Herald: &config.HeraldConfig{
			BaseURL: server.URL,
			Timeout: 1 * time.Second,
		},
		Events: map[string]*config.EventConfig{
			"cert_expired": {
				Type:    "certificate.expired",
				Enabled: true,
			},
		},
	})

	alert := &storage.Alert{
		Type:         "error",
		Title:        "证书已过期",
		ResourceType: strPtr("certificate"),
	}

	// 函数不应该 panic，错误应该被记录到日志
	SendAlertNonBlocking(client, alert, &config.NotificationConfig{})

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)
}
