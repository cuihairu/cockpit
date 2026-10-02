package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Guacamole 目标转发（设计修正 2026-10-02）：guacd 的目标连接不再直拨
// hostname——server 常与目标不同网段（guacd 在云上、目标在 agent 所在内网）。
// 每条会话起一个回环中继 listener：guacd 拨中继，字节流经既有 agent WS 通道
// （proxy_new/proxy_data/proxy_close，与端口转发 proxy/mgr、内置终端
// api_remote 同一条代理管线，agent 侧裸 TCP 拨号分支见 proxy/handler.go
// HandleProxyNew），agent 从自己网络位拨 host:port。hostname 语义随之变为
// 「agent 侧可达的地址」。
//
// 网络拓扑约束：中继绑 server 本机回环，guacd 必须与 server 同机（或共享
// 回环可见性）——与录制目录同路径双挂（GUACD_RECORDING_PATH）是同一前提，
// 见 deployments/guacd/README.md 的部署拓扑。

// guacRelayPrefix Guacamole 转发会话的 proxyId 前缀。server 侧 proxy_data/
// proxy_close/proxy_error 按前缀路由（同 logs:/terminal/vnc 惯例），不进
// proxyMgr（端口转发的 DB 配置面）。
const guacRelayPrefix = "guac:"

// guacRelay 一条 Guacamole 会话的目标转发器
type guacRelay struct {
	agentID string
	proxyID string
	target  string // agent 侧视角的 host:port
	send    func(agentID string, msg *protocol.Message) error

	ln net.Listener

	mu    sync.Mutex
	conns map[string]net.Conn // connID → guacd 侧连接
	seq   atomic.Uint64

	closeOnce sync.Once
}

// guacRelayReg 会话级转发器注册表（proxyID → relay）。包级同 terminalSessions
// 惯例；server.handleProxyData 按前缀路由到这里，不依赖 Server 字段（测试
// 可用零值 Server 走真实分发路径）。
var (
	guacRelayRegMu sync.Mutex
	guacRelayReg   = map[string]*guacRelay{}
)

func guacRelayLookup(proxyID string) *guacRelay {
	guacRelayRegMu.Lock()
	defer guacRelayRegMu.Unlock()
	return guacRelayReg[proxyID]
}

// startGuacRelay 起中继 listener。返回 relay 与「guacd 应拨的地址」
// （127.0.0.1:<ephemeral>，进 connect 指令的 hostname/port）。
func startGuacRelay(agentID, proxyID, target string, send func(string, *protocol.Message) error) (*guacRelay, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("listen relay: %w", err)
	}
	r := &guacRelay{
		agentID: agentID,
		proxyID: proxyID,
		target:  target,
		send:    send,
		ln:      ln,
		conns:   make(map[string]net.Conn),
	}
	guacRelayRegMu.Lock()
	guacRelayReg[proxyID] = r
	guacRelayRegMu.Unlock()
	go r.acceptLoop()
	return r, ln.Addr().String(), nil
}

// Close 幂等关闭：listener + 全部 guacd 侧连接（并逐一通知 agent 拆链）。
func (r *guacRelay) Close() {
	r.closeOnce.Do(func() {
		guacRelayRegMu.Lock()
		delete(guacRelayReg, r.proxyID)
		guacRelayRegMu.Unlock()
		_ = r.ln.Close()
		r.mu.Lock()
		connIDs := make([]string, 0, len(r.conns))
		for id := range r.conns {
			connIDs = append(connIDs, id)
		}
		r.mu.Unlock()
		for _, id := range connIDs {
			r.removeConn(id, "session closed")
		}
	})
}

func (r *guacRelay) acceptLoop() {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return // listener 已关（会话结束）
		}
		go r.handleConn(conn)
	}
}

// handleConn guacd 的一次拨入：注册连接 → 让 agent 拨目标 → 起 guacd→agent 泵。
// 注册先于 proxy_new：agent 回程数据到达时 connID 必已可路由。
func (r *guacRelay) handleConn(conn net.Conn) {
	connID := fmt.Sprintf("%s-%d", r.proxyID, r.seq.Add(1))
	r.mu.Lock()
	r.conns[connID] = conn
	r.mu.Unlock()

	// 裸 TCP 分支：agent 不解析协议内容，协议端点是 guacd（SSH/RDP/VNC 都是）
	newConnMsg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":   r.proxyID,
		"proxyType": "tcp",
		"target":    r.target,
		"connId":    connID,
		"newConn":   true,
	})
	if err := r.send(r.agentID, newConnMsg); err != nil {
		log.Printf("Guacamole relay: proxy_new to agent %s failed: %v", r.agentID, err)
		r.removeConn(connID, "agent unavailable")
		return
	}
	r.pumpToAgent(conn, connID)
}

// pumpToAgent guacd → agent 方向泵。出口（EOF/错误）即拆链并通知 agent。
func (r *guacRelay) pumpToAgent(conn net.Conn, connID string) {
	defer r.removeConn(connID, "guacd closed")
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			dataMsg := protocol.NewMessage(protocol.MessageTypeProxyData, map[string]interface{}{
				"proxyId": r.proxyID,
				"connId":  connID,
				"data":    append([]byte(nil), buf[:n]...),
			})
			if err := r.send(r.agentID, dataMsg); err != nil {
				log.Printf("Guacamole relay: send data to agent %s failed: %v", r.agentID, err)
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("Guacamole relay: read from guacd side %s failed: %v", connID, err)
			}
			return
		}
	}
}

// removeConn 拆一条连接：关 guacd 侧 socket + 通知 agent（幂等——agent 侧
// 先关时会收到对不存在 connID 的 proxy_close，静默忽略）。
func (r *guacRelay) removeConn(connID, reason string) {
	r.mu.Lock()
	conn, ok := r.conns[connID]
	if ok {
		delete(r.conns, connID)
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	log.Printf("Guacamole relay: tearing down %s (%s)", connID, reason)
	_ = conn.Close()
	closeMsg := protocol.NewMessage(protocol.MessageTypeProxyClose, map[string]interface{}{
		"proxyId": r.proxyID,
		"connId":  connID,
		"reason":  reason,
	})
	_ = r.send(r.agentID, closeMsg)
}

// guacRelayDeliver agent → guacd 方向（server.handleProxyData 路由进）。
func guacRelayDeliver(proxyID, connID string, data []byte) error {
	r := guacRelayLookup(proxyID)
	if r == nil {
		return fmt.Errorf("relay %s not found", proxyID)
	}
	r.mu.Lock()
	conn, ok := r.conns[connID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("relay conn %s not found", connID)
	}
	_, err := conn.Write(data)
	if err != nil {
		r.removeConn(connID, "write to guacd failed")
	}
	return err
}

// guacRelayHandleClose agent 侧关闭（目标断开/写错误）→ 关 guacd 侧连接，
// guacd 随之向浏览器走协议层错误路径。
func guacRelayHandleClose(proxyID, connID, reason string) {
	r := guacRelayLookup(proxyID)
	if r == nil {
		return
	}
	r.removeConn(connID, "target closed: "+reason)
}

// guacRelayHandleError agent 拨目标失败（SendError）→ 关 guacd 侧连接，
// 让 guacd 的 connect 立即失败而非挂到超时。
func guacRelayHandleError(proxyID, connID, errMsg string) {
	r := guacRelayLookup(proxyID)
	if r == nil {
		return
	}
	if connID == "" {
		// 无 connId 的错误无法定位连接，整条中继拆掉（guacd connect 必败）
		log.Printf("Guacamole relay: agent error (no connId), closing relay %s: %s", proxyID, errMsg)
		r.Close()
		return
	}
	log.Printf("Guacamole relay: agent dial failed for %s: %s", connID, errMsg)
	r.removeConn(connID, "agent dial failed: "+errMsg)
}
