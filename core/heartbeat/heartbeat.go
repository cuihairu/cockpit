// Package heartbeat 心跳与存活（agent-core ① 通用能力层）：周期组包入队
// 统一上行；未注册静默跳过；发送失败触发重连回调。快照事实（ID/启动时刻/
// 服务面/系统信息）由装配方注入，core 零业务、零平台分支。
package heartbeat

import (
	"context"
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Options 快照事实与处置注入口（均可省：省即对应能力不存在——
// Registered 缺省视为未注册静默跳过）。
type Options struct {
	AgentID    func() string
	StartedAt  func() time.Time
	Services   func() []protocol.RemoteServicePayload
	SystemInfo func() interface{} // nil 不带 systemInfo 字段（原 collector nil 语义）
	Registered func() bool
	Send       func(*protocol.Message) error
	OnSendFail func()
}

// Loop 心跳循环。
type Loop struct {
	ctx context.Context
	o   Options
}

// New 装配。
func New(ctx context.Context, o Options) *Loop {
	return &Loop{ctx: ctx, o: o}
}

// Run 心跳循环直至 ctx 取消。interval 入参在 Run 时读取（包级 var 注入
// 点惯例：测试可在 New 之后、go Run 之前改值）。
func (l *Loop) Run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.Beat()
		case <-l.ctx.Done():
			return
		}
	}
}

// Beat 单次心跳：未注册静默跳过；否则组包入队，失败触发重连回调。
func (l *Loop) Beat() {
	if l.o.Registered == nil || !l.o.Registered() {
		return
	}

	payload := map[string]any{
		"agentId":   l.o.AgentID(),
		"status":    "online",
		"startedAt": l.o.StartedAt().Unix(),
		// 服务面回带缓存（后台重探测，这里不发探测请求）
		"services": l.o.Services(),
	}
	if l.o.SystemInfo != nil {
		if info := l.o.SystemInfo(); info != nil {
			payload["systemInfo"] = info
		}
	}

	if err := l.o.Send(protocol.NewMessage(protocol.MessageTypeHeartbeat, payload)); err != nil {
		log.Printf("Send heartbeat failed: %v", err)
		if l.o.OnSendFail != nil {
			l.o.OnSendFail()
		}
	}
}
