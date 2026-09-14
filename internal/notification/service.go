// Service 多渠道通知扇出（拨测增强，设计见 docs/guide/probe-enhance-design.md）。
//
// 渠道无关的 Notification 载荷 + Channel 接口 + 按配置构建的渠道列表；
// 事件开关沿用 cfg.Events 白名单语义（未配置的事件类型不发送），
// 与 Client.IsEventEnabled 保持一致。
package notification

import (
	"context"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// EventTest 测试通知事件类型（绕过白名单，始终投递到全部渠道）
const EventTest = "test"

// Notification 渠道无关的通知载荷
type Notification struct {
	EventType    string    `json:"event_type"` // service.down / service.up / certificate.expired ...
	Title        string    `json:"title"`
	Message      string    `json:"message"`
	Level        string    `json:"level"` // info / warning / error
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Time         time.Time `json:"time"`
}

// Channel 单个通知渠道
type Channel interface {
	// Name 渠道类型名（herald / ntfy / webhook / telegram）
	Name() string
	// Target 目标摘要（不含 token/secret 等凭据，可安全回显给前端）
	Target() string
	// Send 投递一条通知；实现须短超时、不重试
	Send(ctx context.Context, n *Notification) error
}

// SendResult 单渠道投递结果
type SendResult struct {
	Channel string `json:"channel"`
	Target  string `json:"target,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// Service 多渠道通知服务
type Service struct {
	cfg      *config.NotificationConfig
	channels []Channel
}

// NewService 按配置构建通知服务；cfg 为空或未启用时返回不投递的空服务。
func NewService(cfg *config.NotificationConfig) *Service {
	s := &Service{cfg: cfg}
	if cfg == nil || !cfg.Enabled {
		return s
	}
	if cfg.Herald != nil && cfg.Herald.BaseURL != "" {
		if c := NewClient(cfg); c != nil {
			s.channels = append(s.channels, &heraldChannel{client: c, target: cfg.Herald.BaseURL})
		}
	}
	for _, nc := range cfg.Ntfy {
		if nc != nil && nc.Server != "" && nc.Topic != "" {
			s.channels = append(s.channels, newNtfyChannel(nc))
		}
	}
	for _, wc := range cfg.Webhook {
		if wc != nil && wc.URL != "" {
			s.channels = append(s.channels, newWebhookChannel(wc))
		}
	}
	for _, tc := range cfg.Telegram {
		if tc != nil && tc.BotToken != "" && tc.ChatID != "" {
			s.channels = append(s.channels, newTelegramChannel(tc))
		}
	}
	return s
}

// Enabled 服务是否已启用且有可用渠道
func (s *Service) Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Enabled && len(s.channels) > 0
}

// ChannelSummaries 已启用渠道列表（Name + 目标摘要，不含凭据）
func (s *Service) ChannelSummaries() []SendResult {
	if s == nil {
		return nil
	}
	out := make([]SendResult, 0, len(s.channels))
	for _, ch := range s.channels {
		out = append(out, SendResult{Channel: ch.Name(), Target: ch.Target(), OK: true})
	}
	return out
}

// IsEventEnabled 事件白名单过滤：cfg.Events 里显式 enabled 才投递
func (s *Service) IsEventEnabled(eventType string) bool {
	if !s.Enabled() {
		return false
	}
	for _, eventCfg := range s.cfg.Events {
		if eventCfg != nil && eventCfg.Type == eventType {
			return eventCfg.Enabled
		}
	}
	return false
}

// SendAll 同步扇出到全部渠道（先过事件白名单），返回逐渠道结果
func (s *Service) SendAll(ctx context.Context, n *Notification) []SendResult {
	if s == nil || n == nil || !s.Enabled() {
		return nil
	}
	if !s.IsEventEnabled(n.EventType) {
		return nil
	}
	return s.dispatch(ctx, n)
}

// TestAll 发送测试通知到全部渠道（绕过事件白名单），返回逐渠道结果
func (s *Service) TestAll(ctx context.Context) []SendResult {
	if s == nil || !s.Enabled() {
		return nil
	}
	return s.dispatch(ctx, &Notification{
		EventType: EventTest,
		Title:     "Cockpit 测试通知",
		Message:   "如果你看到这条消息，说明该通知渠道配置正确。",
		Level:     "info",
		Time:      time.Now(),
	})
}

// SendNonBlocking 异步投递（先过事件白名单），仅记录失败日志（告警/探测等异步场景用）
func (s *Service) SendNonBlocking(n *Notification) {
	if s == nil || n == nil || !s.Enabled() {
		return
	}
	if !s.IsEventEnabled(n.EventType) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, r := range s.dispatch(ctx, n) {
			if !r.OK {
				log.Printf("[notification] channel %s (%s) send failed: %s", r.Channel, r.Target, r.Error)
			}
		}
	}()
}

// SendAlertNonBlocking 将存储层 Alert 转为 Notification 后异步投递。
// 事件类型沿用 getAlertEventType 的推导（与 Client.SendAlert 路径一致）。
func (s *Service) SendAlertNonBlocking(alert *storage.Alert) {
	if s == nil || alert == nil || !s.Enabled() {
		return
	}
	n := AlertToNotification(alert)
	if n == nil || n.EventType == "" {
		return
	}
	s.SendNonBlocking(n)
}

// dispatch 并发扇出到全部渠道，忽略事件过滤（过滤在调用方完成）
func (s *Service) dispatch(ctx context.Context, n *Notification) []SendResult {
	results := make([]SendResult, 0, len(s.channels))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, ch := range s.channels {
		wg.Add(1)
		go func(ch Channel) {
			defer wg.Done()
			sendCtx := ctx
			if _, ok := ctx.Deadline(); !ok {
				var cancel context.CancelFunc
				sendCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
			}
			err := ch.Send(sendCtx, n)
			r := SendResult{Channel: ch.Name(), Target: ch.Target(), OK: err == nil}
			if err != nil {
				r.Error = err.Error()
			}
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(ch)
	}
	wg.Wait()
	return results
}

// AlertToNotification 存储层 Alert → 通知载荷；事件类型无法推导时返回 nil。
func AlertToNotification(alert *storage.Alert) *Notification {
	if alert == nil {
		return nil
	}
	n := &Notification{
		Title:   alert.Title,
		Message: alert.Message,
		Level:   alert.Type,
		Time:    time.Now(),
	}
	if alert.ResourceType != nil {
		n.ResourceType = *alert.ResourceType
	}
	if alert.ResourceID != nil {
		n.ResourceID = *alert.ResourceID
	}
	n.EventType = getAlertEventType(alert)
	return n
}

// notificationLabels Herald 兼容的事件标签（与既有 buildEventLabels 字段一致）
func notificationLabels(n *Notification) map[string]string {
	labels := map[string]string{
		"level":   n.Level,
		"title":   n.Title,
		"message": n.Message,
	}
	if n.ResourceID != "" {
		labels["resource_id"] = n.ResourceID
	}
	if n.ResourceType != "" {
		labels["resource_type"] = n.ResourceType
	}
	return labels
}

// targetHost 从 URL 提取 host 作为目标摘要；解析失败返回原文截断。
func targetHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if len(raw) > 40 {
			return raw[:40]
		}
		return raw
	}
	return u.Host
}
