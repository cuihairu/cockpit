package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/gorilla/websocket"
)

// Handler Agent 端代理处理器
type Handler struct {
	mu       sync.RWMutex
	conn     *websocket.Conn
	conns    map[string]*AgentTargetConn // connID -> AgentTargetConn
	connSeq  atomic.Uint64
	sendFunc func(*protocol.Message) error
	running  atomic.Bool
}

// AgentTargetConn Agent 端的连接。
// Conn 抽象为 io.ReadWriteCloser：裸 TCP 目标是 net.Conn，SSH 目标是
// PTY 会话适配流（sshStream），二者共用 readFromTarget 转发管道。
type AgentTargetConn struct {
	ID      string
	ProxyID string
	Target  string
	Conn    io.ReadWriteCloser
	Created time.Time
	// sshSess 非空时表示 SSH 终端会话，用于 PTY WindowChange
	sshSess *SSHSession
	mu      sync.RWMutex
	closed  atomic.Bool
}

// windowResizer 可调整终端窗口尺寸的目标连接（SSH PTY）。
type windowResizer interface {
	WindowChange(rows, cols int) error
}

// NewHandler 创建 Agent 端代理处理器
func NewHandler() *Handler {
	return &Handler{
		conns: make(map[string]*AgentTargetConn),
	}
}

// Start 启动处理器并绑定 websocket 连接。
// 多次调用安全：conn 字段每次都会更新（用于 Agent 重连场景）。
func (h *Handler) Start(wsConn *websocket.Conn) {
	h.AttachConn(wsConn)
	h.running.Store(true)
	log.Println("Agent proxy handler started")
}

// SetSendFunc 设置发送函数。Agent 注入统一 writer，避免多 goroutine 并发写 websocket。
func (h *Handler) SetSendFunc(fn func(*protocol.Message) error) {
	h.mu.Lock()
	h.sendFunc = fn
	h.mu.Unlock()
}

// AttachConn 更新当前 websocket 连接（Agent 重连后调用以替换旧 conn）
func (h *Handler) AttachConn(wsConn *websocket.Conn) {
	h.mu.Lock()
	h.conn = wsConn
	h.mu.Unlock()
}

// Stop 停止处理器
func (h *Handler) Stop() {
	if !h.running.CompareAndSwap(true, false) {
		return
	}

	// 关闭所有连接
	h.mu.Lock()
	for _, conn := range h.conns {
		conn.Close()
	}
	h.conns = make(map[string]*AgentTargetConn)
	h.mu.Unlock()

	log.Println("Agent proxy handler stopped")
}

// HandleProxyNew 处理新建代理连接请求
func (h *Handler) HandleProxyNew(msg *protocol.Message) error {
	p, err := protocol.DecodeProxyNew(msg)
	if err != nil {
		return fmt.Errorf("invalid proxy new message: %w", err)
	}

	if p.ProxyID == "" || p.Target == "" || p.ConnID == "" {
		return fmt.Errorf("invalid proxy new message: missing required fields")
	}

	// SSH 终端：agent 侧终结 SSH 协议（Dial → 认证 → PTY → Shell），
	// 之后的 proxy_data 双向流即 PTY 字节流。telnet 等仍走裸 TCP。
	if p.Terminal && p.Protocol == string(protocol.RemoteProtocolSSH) {
		return h.openSSHProxy(p)
	}

	// 裸 TCP 目标（telnet / 普通 TCP 代理）
	targetConn, err := net.DialTimeout("tcp", p.Target, 10*time.Second)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", p.Target, err)
		// 发送错误消息给 Server
		h.SendError(p.ProxyID, p.ConnID, err.Error())
		return err
	}

	agentConn := &AgentTargetConn{
		ID:      p.ConnID,
		ProxyID: p.ProxyID,
		Target:  p.Target,
		Conn:    targetConn,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns[p.ConnID] = agentConn
	h.mu.Unlock()

	log.Printf("Agent: New proxy connection %s -> %s", p.ConnID, p.Target)

	// 启动数据读取协程
	go h.readFromTarget(agentConn)

	return nil
}

// openSSHProxy 建立 SSH 终端代理：SSH 会话即 Conn，转发管道复用裸 TCP 路径。
func (h *Handler) openSSHProxy(p protocol.ProxyNewPayload) error {
	sshSess, err := NewSSHSession(p.Target, p.Username, p.Password, p.PrivateKey, 24, 80)
	if err != nil {
		log.Printf("Failed to establish SSH session to %s: %v", p.Target, err)
		h.SendError(p.ProxyID, p.ConnID, err.Error())
		return err
	}

	agentConn := &AgentTargetConn{
		ID:      p.ConnID,
		ProxyID: p.ProxyID,
		Target:  p.Target,
		Conn:    sshSess.Stream(),
		Created: time.Now(),
		sshSess: sshSess,
	}

	h.mu.Lock()
	h.conns[p.ConnID] = agentConn
	h.mu.Unlock()

	log.Printf("Agent: New SSH proxy session %s -> %s", p.ConnID, p.Target)
	go h.readFromTarget(agentConn)
	return nil
}

// HandleProxyData 处理来自 Server 的数据
func (h *Handler) HandleProxyData(msg *protocol.Message) error {
	p, err := protocol.DecodeProxyData(msg)
	if err != nil {
		return fmt.Errorf("invalid proxy data message: %w", err)
	}

	h.mu.RLock()
	conn, exists := h.conns[p.ConnID]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("connection %s not found", p.ConnID)
	}

	if conn.closed.Load() {
		return fmt.Errorf("connection %s already closed", p.ConnID)
	}

	// 终端窗口尺寸变更：调 PTY WindowChange，不写数据。
	// SSH 会话的 WindowChange 在 SSHSession 上（Stream 是纯 IO 适配器，
	// 不实现 windowResizer），优先走 sshSess；其余目标按接口探测。
	if p.Resize {
		var rz windowResizer
		if conn.sshSess != nil {
			rz = conn.sshSess
		} else if r, ok := conn.Conn.(windowResizer); ok {
			rz = r
		} else {
			return nil // 裸 TCP 目标无 PTY 概念，静默忽略
		}
		if err := rz.WindowChange(p.Rows, p.Cols); err != nil {
			log.Printf("Window change error on %s: %v", p.ConnID, err)
		}
		return nil
	}

	// 写入数据到目标
	if _, err := conn.Conn.Write(p.Data); err != nil {
		log.Printf("Write error to target %s: %v", p.ConnID, err)
		conn.Close()
		h.SendClose(conn.ProxyID, conn.ID, "write error")
		return err
	}

	return nil
}

// HandleProxyClose 处理来自 Server 的关闭连接请求
func (h *Handler) HandleProxyClose(msg *protocol.Message) error {
	p, err := protocol.DecodeProxyClose(msg)
	if err != nil {
		return fmt.Errorf("invalid proxy close message: %w", err)
	}

	h.mu.Lock()
	conn, exists := h.conns[p.ConnID]
	if exists {
		delete(h.conns, p.ConnID)
	}
	h.mu.Unlock()

	if exists {
		log.Printf("Agent: Closing connection %s", p.ConnID)
		conn.Close()
	}

	return nil
}

// readFromTarget 从目标读取数据并发送给 Server
func (h *Handler) readFromTarget(conn *AgentTargetConn) {
	defer conn.Close()

	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := conn.Conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error from target %s: %v", conn.ID, err)
			}
			break
		}

		// 发送数据给 Server
		dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
			"proxyId": conn.ProxyID,
			"connId":  conn.ID,
			"data":    buf[:n],
		})

		if err := h.SendMessage(dataMsg); err != nil {
			log.Printf("Failed to send data to server: %v", err)
			break
		}
	}

	// 通知 Server 连接关闭
	h.SendClose(conn.ProxyID, conn.ID, "target closed")
}

// SendMessage 发送消息给 Server
func (h *Handler) SendMessage(msg *protocol.Message) error {
	if !h.running.Load() {
		return fmt.Errorf("handler not running")
	}

	h.mu.RLock()
	sendFunc := h.sendFunc
	h.mu.RUnlock()
	if sendFunc == nil {
		return fmt.Errorf("send function not configured")
	}
	return sendFunc(msg)
}

// SendError 发送错误消息
func (h *Handler) SendError(proxyID, connID, errMsg string) {
	msg := protocol.NewMessage(protocol.MessageTypeProxyError, map[string]interface{}{
		"proxyId": proxyID,
		"connId":  connID,
		"error":   errMsg,
	})
	h.SendMessage(msg)
}

// SendClose 发送关闭消息
func (h *Handler) SendClose(proxyID, connID, reason string) {
	msg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": proxyID,
		"connId":  connID,
		"reason":  reason,
	})
	h.SendMessage(msg)
}

// Close 关闭代理连接
func (ac *AgentTargetConn) Close() error {
	if !ac.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}
	if ac.sshSess != nil {
		// SSH：关会话（PTY/shell）+ 底层连接，幂等
		return ac.sshSess.Close()
	}
	return ac.Conn.Close()
}
