package mgr

import (
	"net"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

// TestS3NewConnectionNotCleanedByFirstTick 验证 S3：新连接（未发首字节）
// 创建时必须初始化 LastRead，首个 cleanup tick 不得因 now.Sub(zero) 超大
// 而误关握手中的活动连接。
func TestS3NewConnectionNotCleanedByFirstTick(t *testing.T) {
	mockServer := newMockServerInterface()
	db := testDB(t)
	manager := NewManager(mockServer, db)

	proxyConfig := &storage.Proxy{
		ID:         "s3-proxy",
		Name:       "S3 Test",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 0,
		Target:     "localhost:8080",
		Enabled:    true,
	}
	if err := manager.StartProxy(proxyConfig); err != nil {
		t.Fatal(err)
	}
	defer manager.StopProxy(proxyConfig.ID)

	clientConn, targetConn := net.Pipe()
	defer targetConn.Close()

	// 走真实 handleConnection：内部构造 ProxyConn 并挂入 conns，再起读协程。
	manager.mu.RLock()
	proxy := manager.proxies[proxyConfig.ID]
	manager.mu.RUnlock()
	manager.handleConnection(proxy, clientConn)

	manager.mu.RLock()
	p := manager.proxies[proxyConfig.ID]
	manager.mu.RUnlock()
	if len(p.conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(p.conns))
	}

	// 立即执行清理：新连接 LastRead 刚初始化（near now），不应被误删。
	manager.cleanupIdleConnections()

	manager.mu.RLock()
	p = manager.proxies[proxyConfig.ID]
	n := len(p.conns)
	manager.mu.RUnlock()
	if n != 1 {
		t.Fatalf("new connection was wrongly cleaned: %d conns, want 1 (S3)", n)
	}
}
