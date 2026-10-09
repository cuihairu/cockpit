// Package report 统一上行（agent-core ① 通用能力层）。全部子模块的出站
// 消息经 Enqueue 排队，Run 单线程串行落线——gorilla 同一连接禁止并发
// writer。注册放行门、连接事实与写失败处置由装配方注入，core 零业务、
// 零平台分支。依赖 internal/protocol 为架构文档 §8 白名单例外（协议一份）。
package report

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// pollInterval 注册放行门轮询周期（原 agent.go waitRegistered 同值）。
var pollInterval = 100 * time.Millisecond

// Upstream 统一上行。
type Upstream struct {
	ctx   context.Context
	queue chan *protocol.Message
	codec *protocol.Codec
	// conn 连接事实快照（装配方在锁内取当前 conn，Stop/reconnect 可并发
	// 置 nil——裸读会传 nil 进 ReadMessage/WriteMessage）
	conn func() *websocket.Conn
	// lock 连接级写互斥：队列写与注册首包直写共享同一把
	lock *sync.Mutex
	// registered 注册放行门（Run 消费消息前轮询）
	registered func() bool
	// onWriteErr 写失败处置（agent 侧：复位 registered + 触发重连）
	onWriteErr func()
}

// New 装配。queue 由装配方持有并传入（测试可预置/观测），容量语义不变。
func New(ctx context.Context, queue chan *protocol.Message, codec *protocol.Codec,
	conn func() *websocket.Conn, lock *sync.Mutex,
	registered func() bool, onWriteErr func()) *Upstream {
	return &Upstream{
		ctx:        ctx,
		queue:      queue,
		codec:      codec,
		conn:       conn,
		lock:       lock,
		registered: registered,
		onWriteErr: onWriteErr,
	}
}

// Enqueue 业务消息入队（不阻塞调用方：停机或队满即报错）。
func (u *Upstream) Enqueue(msg *protocol.Message) error {
	select {
	case u.queue <- msg:
		return nil
	case <-u.ctx.Done():
		return fmt.Errorf("agent stopped")
	default:
		return fmt.Errorf("agent outbound queue full")
	}
}

// WriteNow 不经队列直写（注册首包走这里：注册完成前 Run 的放行门不会
// 放行任何队列消息）。
func (u *Upstream) WriteNow(msg *protocol.Message) error {
	conn := u.conn()
	if conn == nil {
		return fmt.Errorf("agent not connected")
	}
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.codec.WriteMessage(conn, msg)
}

// Run 消费队列直至 ctx 取消：每条消息先过注册放行门，写出失败回调
// onWriteErr 后继续消费（下一条消息在放行门等待重注册，原 writeLoop
// 语义——写失败不终止循环，由重连侧恢复注册）。
func (u *Upstream) Run() {
	for {
		select {
		case <-u.ctx.Done():
			return
		case msg := <-u.queue:
			if err := WaitRegistered(u.ctx, u.registered); err != nil {
				return
			}
			if err := u.write(msg); err != nil {
				log.Printf("Send message failed: %v", err)
				if u.onWriteErr != nil {
					u.onWriteErr()
				}
			}
		}
	}
}

// write 锁内串行写出。
func (u *Upstream) write(msg *protocol.Message) error {
	conn := u.conn()
	if conn == nil {
		return fmt.Errorf("agent not connected")
	}
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.codec.WriteMessage(conn, msg)
}

// WaitRegistered 注册放行门：轮询 registered 直到放行或 ctx 取消。
func WaitRegistered(ctx context.Context, registered func() bool) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if registered() {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("agent stopped")
		case <-ticker.C:
		}
	}
}
