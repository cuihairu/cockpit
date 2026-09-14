package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

// webhookChannel 通用 webhook 渠道：POST 完整通知载荷的 JSON，
// 可选 X-Cockpit-Secret 头供接收方鉴权。
type webhookChannel struct {
	cfg    *config.WebhookConfig
	client *http.Client
}

func newWebhookChannel(cfg *config.WebhookConfig) *webhookChannel {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &webhookChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *webhookChannel) Name() string   { return "webhook" }
func (c *webhookChannel) Target() string { return targetHost(c.cfg.URL) }

func (c *webhookChannel) Send(ctx context.Context, n *Notification) error {
	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Secret != "" {
		req.Header.Set("X-Cockpit-Secret", c.cfg.Secret)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	return nil
}
