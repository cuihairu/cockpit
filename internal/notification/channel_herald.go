package notification

import (
	"context"
	"fmt"
)

// heraldChannel 既有 Herald 服务的渠道适配：复用 Client.SendEvent，
// 请求体 {type, labels} 与历史行为完全一致。
type heraldChannel struct {
	client *Client
	target string
}

func (h *heraldChannel) Name() string   { return "herald" }
func (h *heraldChannel) Target() string { return targetHost(h.target) }

func (h *heraldChannel) Send(ctx context.Context, n *Notification) error {
	if h.client == nil {
		return errChannelNotConfigured("herald")
	}
	return h.client.SendEvent(ctx, &Event{
		Type:   n.EventType,
		Labels: notificationLabels(n),
	})
}

// errChannelNotConfigured 渠道配置缺失的统一错误
func errChannelNotConfigured(name string) error {
	return fmt.Errorf("%s channel not configured", name)
}
