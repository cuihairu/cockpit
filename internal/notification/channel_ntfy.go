package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

// ntfyChannel ntfy 渠道：JSON 发布模式（POST {server}/，body 携带 topic），
// 避免 UTF-8 标题放进 HTTP header 的编码问题。
type ntfyChannel struct {
	cfg    *config.NtfyConfig
	client *http.Client
}

func newNtfyChannel(cfg *config.NtfyConfig) *ntfyChannel {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ntfyChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *ntfyChannel) Name() string   { return "ntfy" }
func (c *ntfyChannel) Target() string { return c.cfg.Topic }

// ntfyPriority 按通知级别推导 ntfy 优先级（1 低 – 5 紧急），配置显式指定时以配置为准
func (c *ntfyChannel) priority(n *Notification) string {
	if c.cfg.Priority != "" {
		return c.cfg.Priority
	}
	switch n.Level {
	case "error":
		return "high"
	case "warning":
		return "default"
	default:
		return "low"
	}
}

func (c *ntfyChannel) Send(ctx context.Context, n *Notification) error {
	body := map[string]interface{}{
		"topic":    c.cfg.Topic,
		"title":    n.Title,
		"message":  n.Message,
		"priority": c.priority(n),
		"tags":     []string{n.Level, "cockpit"},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal ntfy payload: %w", err)
	}

	url := strings.TrimSuffix(c.cfg.Server, "/") + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
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
