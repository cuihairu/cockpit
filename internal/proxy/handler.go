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
	conn      *websocket.Conn
	conns     map[string]*AgentTargetConn // connID -> AgentTargetConn
	connSeq   atomic.Uint64
	mu        sync.RWMutex
	sendQueue chan *protocol.Message
	running   atomic.Bool
}

// AgentTargetConn Agent 端的连接
type AgentTargetConn struct {
	ID        string
	ProxyID   string
	Target    string
	Conn      net.Conn
	Created   time.Time
	mu        sync.RWMutex
	closed    atomic.Bool
}

// NewHandler 创建 Agent 端代理处理器
func NewHandler() *Handler {
	return &Handler{
		conns:     make(map[string]*AgentTargetConn),
		sendQueue: make(chan *protocol.Message, 1000),
	}
}

// Start 启动处理器
func (h *Handler) Start(wsConn *websocket.Conn) {
	if !h.running.CompareAndSwap(false, true) {
		return
	}

	h.conn = wsConn

	// 启动发送协程
	go h.sendLoop()

	log.Println("Agent proxy handler started")
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

	close(h.sendQueue)

	log.Println("Agent proxy handler stopped")
}

// HandleProxyNew 处理新建代理连接请求
func (h *Handler) HandleProxyNew(msg *protocol.Message) error {
	proxyID, _ := msg.Payload["proxyId"].(string)
	_, _ = msg.Payload["proxyType"].(string) // 预留，目前只支持 TCP
	target, _ := msg.Payload["target"].(string)
	connID, _ := msg.Payload["connId"].(string)

	if proxyID == "" || target == "" || connID == "" {
		return fmt.Errorf("invalid proxy new message: missing required fields")
	}

	// 连接到目标服务
	targetConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", target, err)
		// 发送错误消息给 Server
		h.SendError(proxyID, connID, err.Error())
		return err
	}

	agentConn := &AgentTargetConn{
		ID:      connID,
		ProxyID: proxyID,
		Target:  target,
		Conn:    targetConn,
		Created: time.Now(),
	}

	h.mu.Lock()
	h.conns[connID] = agentConn
	h.mu.Unlock()

	log.Printf("Agent: New proxy connection %s -> %s", connID, target)

	// 启动数据读取协程
	go h.readFromTarget(agentConn)

	return nil
}

// HandleProxyData 处理来自 Server 的数据
func (h *Handler) HandleProxyData(msg *protocol.Message) error {
	connID, _ := msg.Payload["connId"].(string)
	dataBytes, ok := msg.Payload["data"]
	if !ok {
		return fmt.Errorf("missing data in proxy data message")
	}

	// 解码 Base64 数据（因为 JSON 传输）
	var data []byte
	switch v := dataBytes.(type) {
	case string:
		// JSON 字符串，需要解码
		data = []byte(v)
	case []byte:
		data = v
	case []interface{}:
		// JSON 数组
		data = make([]byte, len(v))
		for i, b := range v {
			if f, ok := b.(float64); ok {
				data[i] = byte(f)
			}
		}
	default:
		return fmt.Errorf("invalid data type: %T", dataBytes)
	}

	h.mu.RLock()
	conn, exists := h.conns[connID]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("connection %s not found", connID)
	}

	if conn.closed.Load() {
		return fmt.Errorf("connection %s already closed", connID)
	}

	// 写入数据到目标
	_, err := conn.Conn.Write(data)
	if err != nil {
		log.Printf("Write error to target %s: %v", connID, err)
		conn.Close()
		h.SendClose(conn.ProxyID, conn.ID, "write error")
		return err
	}

	return nil
}

// HandleProxyClose 处理来自 Server 的关闭连接请求
func (h *Handler) HandleProxyClose(msg *protocol.Message) error {
	connID, _ := msg.Payload["connId"].(string)

	h.mu.Lock()
	conn, exists := h.conns[connID]
	if exists {
		delete(h.conns, connID)
	}
	h.mu.Unlock()

	if exists {
		log.Printf("Agent: Closing connection %s", connID)
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

	select {
	case h.sendQueue <- msg:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// sendLoop 发送循环
func (h *Handler) sendLoop() {
	for {
		select {
		case msg, ok := <-h.sendQueue:
			if !ok {
				return
			}
			if err := h.conn.WriteJSON(msg); err != nil {
				log.Printf("Failed to send message: %v", err)
				return
			}
		}
	}
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
	return ac.Conn.Close()
}
