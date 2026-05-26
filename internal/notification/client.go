package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Client Herald 通知客户端
type Client struct {
	config     *config.NotificationConfig
	httpClient *http.Client
}

// NewClient 创建新的通知客户端
func NewClient(cfg *config.NotificationConfig) *Client {
	if cfg == nil {
		return nil
	}

	timeout := 10 * time.Second
	if cfg.Herald != nil && cfg.Herald.Timeout > 0 {
		timeout = cfg.Herald.Timeout
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SendEvent 发送事件到 Herald
func (c *Client) SendEvent(ctx context.Context, event *Event) error {
	if c == nil || c.config == nil || c.config.Herald == nil {
		return fmt.Errorf("client or herald config is nil")
	}

	if event == nil {
		return nil
	}

	if c.config.Herald.BaseURL == "" {
		return fmt.Errorf("herald base URL is empty")
	}

	// 构建请求体
	reqBody := map[string]interface{}{
		"type":   event.Type,
		"labels": event.Labels,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// 创建请求
	url := c.config.Herald.BaseURL + "/api/v1/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendAlert 将 Alert 转换为事件并发送
func (c *Client) SendAlert(ctx context.Context, alert *storage.Alert) error {
	if c == nil || c.config == nil {
		return fmt.Errorf("client or config is nil")
	}

	event := AlertToEvent(alert, c.config)
	if event == nil {
		return nil // 无需发送的事件
	}

	return c.SendEvent(ctx, event)
}

// IsEnabled 检查通知是否启用
func (c *Client) IsEnabled() bool {
	return c != nil && c.config != nil && c.config.Enabled
}

// IsEventEnabled 检查特定事件类型是否启用
func (c *Client) IsEventEnabled(eventType string) bool {
	if !c.IsEnabled() {
		return false
	}

	for _, eventCfg := range c.config.Events {
		if eventCfg.Type == eventType {
			return eventCfg.Enabled
		}
	}

	return false
}

// SendAlertNonBlocking 非阻塞发送警告（用于异步场景）
func SendAlertNonBlocking(client *Client, alert *storage.Alert, cfg *config.NotificationConfig) {
	if client == nil || !client.IsEnabled() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := client.SendAlert(ctx, alert); err != nil {
			log.Printf("[notification] failed to send alert: %v", err)
		}
	}()
}
