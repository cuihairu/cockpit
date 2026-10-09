package probeagent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuihairu/cockpit/core/backoff"
	"github.com/cuihairu/cockpit/core/heartbeat"
	"github.com/cuihairu/cockpit/core/platform"
	"github.com/cuihairu/cockpit/core/register"
	"github.com/cuihairu/cockpit/core/report"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// 上行链计时（简档 §4.2：迁移即发 + 每 10×interval 全量兜底）。
var (
	upstreamHeartbeatInterval = 30 * time.Second
	upstreamReconnectDelay    = 5 * time.Second
	upstreamReconnectMaxDelay = 10 * time.Second
	upstreamFallbackFloor     = 30 * time.Second
	upstreamFallbackCeiling   = 10 * time.Minute
)

// upstreamOutboundQueue 上行队列容量（报告是小消息，128 足够断线缓冲）。
const upstreamOutboundQueue = 128

// DeriveProbeID 探针 ID：显式 agent_id 优先；否则 probe-<hostname>[-<machine-id
// 前 8>]。前缀与主机 agent 的 agent- 区分——同机双 agent 撞 ID 会互踢注册。
func DeriveProbeID(explicit, hostname, machineID string) string {
	if explicit != "" {
		return explicit
	}
	id := "probe-" + hostname
	if len(machineID) > 8 {
		machineID = machineID[:8]
	}
	if machineID != "" {
		id += "-" + machineID
	}
	return id
}

// Upstream server 上行链（B5b）：精简连接生命周期——拨号、注册、心跳、
// probe_report 上行、读循环、断线重连。复用 core 三件（register/heartbeat/
// report），同 internal/agent 装配模式；无能力探测/无 RPC provider/无代理。
type Upstream struct {
	a         *Agent
	serverURL string
	secret    string
	version   string

	mu         sync.RWMutex
	conn       *websocket.Conn
	writeMu    sync.Mutex
	registered atomic.Bool
	resolvedID string
	startedAt  time.Time

	ctx      context.Context
	cancel   context.CancelFunc
	stopOnce sync.Once

	outbound   chan *protocol.Message
	upstream   *report.Upstream
	registrar  *register.Registrar
	heartbeats *heartbeat.Loop
	codec      *protocol.Codec

	reconnecting atomic.Bool
	// notify 上行信号：迁移即发与兜底 ticker 都触发一次全量上报。
	// Alert 走 Guard 锁内回调链，Notify 只入队不回读（同 Alerter 契约）。
	notify chan struct{}
	// fallbackInterval 兜底上报周期（构造期按 10×全局 interval 夹定）。
	fallbackInterval time.Duration
	// reconnectInitial/Max 重连退避参数（构造期自包级缺省快照——测试注入
	// 包级变量须在 NewUpstream 之前；读字段使在途 goroutine 与测试参数
	// 还原之间不存在数据竞争面）。
	reconnectInitial time.Duration
	reconnectMax     time.Duration
}

// NewUpstream 装配上行链（cfg.Server 非空才有意义；once 模式不装配）。
func NewUpstream(a *Agent, cfg *Config, version string) *Upstream {
	fallback := clampFallbackInterval(cfg.Interval.D() * 10)
	ctx, cancel := context.WithCancel(context.Background())
	u := &Upstream{
		a:         a,
		serverURL: cfg.Server,
		secret:    cfg.Secret,
		version:   version,
		startedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		outbound:  make(chan *protocol.Message, upstreamOutboundQueue),
		codec:     protocol.NewCodec(),
		notify:    make(chan struct{}, 1),

		fallbackInterval: fallback,

		reconnectInitial: upstreamReconnectDelay,
		reconnectMax:     upstreamReconnectMaxDelay,
	}
	u.resolvedID = DeriveProbeID(cfg.AgentID, u.hostname(), u.machineID())

	// 统一上行（core/report）：连接事实/注册门/写失败处置注入
	u.upstream = report.New(ctx, u.outbound, u.codec,
		func() *websocket.Conn {
			u.mu.RLock()
			defer u.mu.RUnlock()
			return u.conn
		},
		&u.writeMu,
		func() bool { return u.registered.Load() },
		func() {
			u.registered.Store(false)
			go u.reconnect()
		})
	// 注册上线（core/register）：探针元数据 + 传输注入
	u.registrar = register.New(
		register.Config{
			AgentID:  u.resolvedID,
			Secret:   cfg.Secret,
			Version:  version,
			Metadata: map[string]interface{}{"role": "probe"},
		},
		register.Facts{MachineID: u.machineID},
		func(m *protocol.Message) error { return u.upstream.WriteNow(m) },
		u.readRegisterResp,
		func() { u.registered.Store(true) })
	// 心跳（core/heartbeat）：探针无服务面/系统信息注入（Services 空闭包
	// 必须显式给——nil func 在 Beat 组包处直接 panic）
	u.heartbeats = heartbeat.New(ctx, heartbeat.Options{
		AgentID:    func() string { return u.resolvedID },
		StartedAt:  func() time.Time { return u.startedAt },
		Services:   func() []protocol.RemoteServicePayload { return nil },
		Registered: func() bool { return u.registered.Load() },
		Send:       u.upstream.Enqueue,
		OnSendFail: func() { go u.reconnect() },
	})
	return u
}

// hostname 主机名（派生 ID 用；取不到进 ID 字符串也无碍）。
func (u *Upstream) hostname() string {
	h, _ := os.Hostname()
	return h
}

// machineID 平台机器标识（platform 层装配缺省可 nil，同 machineID 空串语义）。
func (u *Upstream) machineID() string {
	h := platform.Current()
	if h == nil {
		return ""
	}
	return h.MachineID()
}

// Registered 注册放行门观测（测试用）。
func (u *Upstream) Registered() bool { return u.registered.Load() }

// ID 派生后的探针 ID。
func (u *Upstream) ID() string { return u.resolvedID }

// Notify 请求立即上行一次全量报告（迁移即发）。Guard 锁内回调路径安全：
// 只入队信号，不回读 Agent/Guard 状态（同 Alerter 契约）。
func (u *Upstream) Notify() {
	select {
	case u.notify <- struct{}{}:
	default:
	}
}

// clampFallbackInterval 兜底周期夹在 [30s, 10m]：interval 过小防打爆，
// 过大防失联窗口失控。
func clampFallbackInterval(d time.Duration) time.Duration {
	if d < upstreamFallbackFloor {
		d = upstreamFallbackFloor
	}
	if d > upstreamFallbackCeiling {
		d = upstreamFallbackCeiling
	}
	return d
}

// Run 阻塞运行上行链至 Stop：拨号→注册→写循环/心跳/读循环/上报 worker；
// 断线按退避重连。返回值恒 nil（错误只在日志，恢复交给重连）。
func (u *Upstream) Run() error {
	if err := u.connect(); err != nil {
		log.Printf("probe upstream connect failed: %v", err)
		go u.reconnect()
	} else if err := u.register(); err != nil {
		if u.ctx.Err() != nil {
			return nil
		}
		log.Printf("probe upstream register failed: %v", err)
		u.closeConn()
		go u.reconnect()
	}
	go u.upstream.Run()
	go u.heartbeats.Run(upstreamHeartbeatInterval)
	go u.messageLoop()
	go u.reportWorker()
	<-u.ctx.Done()
	return nil
}

// Stop 停上行链（幂等）：取消 ctx + 关连接。
func (u *Upstream) Stop() {
	u.stopOnce.Do(func() {
		u.cancel()
		u.closeConn()
	})
}

// connect 拨号（wss 强制 HTTP/1.1，同主 agent——ALPN 协商到 h2 会被
// 反代剥 Upgrade 头致 400）。
func (u *Upstream) connect() error {
	log.Printf("probe upstream connecting to %s...", u.serverURL)
	dialer := *websocket.DefaultDialer // 值拷贝，严禁改全局单例
	dialer.HandshakeTimeout = 10 * time.Second
	if dialer.TLSClientConfig == nil {
		dialer.TLSClientConfig = &tls.Config{}
	}
	dialer.TLSClientConfig.NextProtos = []string{"http/1.1"}

	conn, _, err := dialer.Dial(u.serverURL, nil)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.conn = conn
	u.registered.Store(false)
	u.mu.Unlock()
	return nil
}

// register 注册上线（core/register：报文/首包直写/握手/放行门）。
func (u *Upstream) register() error {
	_, _, err := u.registrar.Register(u.startedAt, []protocol.Capability{{Type: "probe"}}, nil)
	return err
}

// readRegisterResp 注册应答读：锁内快照 conn——Stop/reconnect 可并发把
// conn 置 nil，裸读会把 nil 传进 ReadMessage 导致 panic。
func (u *Upstream) readRegisterResp() (*protocol.Message, error) {
	u.mu.RLock()
	conn := u.conn
	u.mu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("probe agent not connected")
	}
	return u.codec.ReadMessage(conn)
}

// closeConn 关当前连接并复位注册门。
func (u *Upstream) closeConn() {
	u.mu.Lock()
	conn := u.conn
	u.conn = nil
	u.registered.Store(false)
	u.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// reconnect 断线重连：CAS 防并发多循环；首等 reconnectDelay，失败恒按
// 封顶间隔重试（core/backoff），等待可被 Stop 中断。不另起读循环——
// messageLoop 全生命周期单实例，重连成功后自行快照到新连接。
func (u *Upstream) reconnect() {
	if !u.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer u.reconnecting.Store(false)

	u.closeConn()
	it := backoff.Start(backoff.Policy{Initial: u.reconnectInitial, Max: u.reconnectMax})
	for {
		if !backoff.Wait(u.ctx, it.Next()) {
			return
		}
		if err := u.connect(); err != nil {
			log.Printf("probe upstream reconnect failed: %v", err)
			continue
		}
		if err := u.register(); err != nil {
			log.Printf("probe upstream re-register failed: %v", err)
			u.closeConn()
			continue
		}
		log.Printf("probe upstream reconnected")
		return
	}
}

// messageLoop 全生命周期唯一读循环（gorilla 同连接禁止并发读者——读错误
// 交 reconnect 后回快照循环等新连接，reconnect 不另起）：注册应答期
// （registered=false）让位 registrar 独读，避免抢走注册响应。消费服务器
// 下行（ping→pong、RPC 拒答——探针无 provider，快错快显）。
func (u *Upstream) messageLoop() {
	for {
		u.mu.RLock()
		conn := u.conn
		u.mu.RUnlock()
		if conn == nil || !u.registered.Load() {
			select {
			case <-u.ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		msg, err := u.codec.ReadMessage(conn)
		if err != nil {
			select {
			case <-u.ctx.Done():
				return
			default:
			}
			log.Printf("probe upstream read failed: %v", err)
			go u.reconnect()
			// 让位重连：等 closeConn 复位后再回快照循环，避免对断连热读
			select {
			case <-u.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		u.handleMessage(msg)
	}
}

// handleMessage 服务器下行分发。
func (u *Upstream) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypePing:
		resp := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
			"status":     "pong",
			"serverTime": time.Now().Unix(),
		})
		resp.ID = msg.ID
		if err := u.upstream.Enqueue(resp); err != nil {
			log.Printf("probe upstream pong: %v", err)
		}
	case protocol.MessageTypeRPCRequest:
		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "error",
			"error":  "probe agent has no rpc providers",
		})
		resp.ID = msg.ID
		if err := u.upstream.Enqueue(resp); err != nil {
			log.Printf("probe upstream rpc reject: %v", err)
		}
	default:
		// 心跳 ack / 注册 ack（重连时由 registrar 直读）静默
	}
}

// reportWorker 上报 worker：迁移即发（notify 信号）与 10×interval 兜底
// 都发全量报告；未注册时经 core/report 放行门排队，注册后落线。
func (u *Upstream) reportWorker() {
	fallback := time.NewTicker(u.fallbackInterval)
	defer fallback.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-u.notify:
		case <-fallback.C:
		}
		if err := u.upstream.Enqueue(u.a.UpstreamMessage()); err != nil {
			log.Printf("probe upstream report: %v", err)
		}
	}
}
