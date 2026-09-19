package notification

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
)

// telegramChannel Telegram Bot 渠道：sendMessage + HTML 解析模式
type telegramChannel struct {
	cfg    *config.TelegramConfig
	client *http.Client
}

func newTelegramChannel(cfg *config.TelegramConfig) *telegramChannel {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &telegramChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *telegramChannel) Name() string   { return "telegram" }
func (c *telegramChannel) Target() string { return c.cfg.ChatID }

func (c *telegramChannel) Send(ctx context.Context, n *Notification) error {
	text := "<b>" + html.EscapeString(n.Title) + "</b>\n" + html.EscapeString(n.Message)
	if n.ResourceType != "" {
		text += "\n\n资源: " + html.EscapeString(n.ResourceType+" "+n.ResourceID)
	}
	body := map[string]interface{}{
		"chat_id":    c.cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	data, err := jsonMarshal(body)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	url := "https://api.telegram.org/bot" + c.cfg.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
