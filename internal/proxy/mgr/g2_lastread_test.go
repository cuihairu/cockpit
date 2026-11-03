package mgr

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestProxyConnLastReadConcurrentAccess 验证 G2：LastRead 的写（markRead，
// 读协程）与读（lastReadAt，cleanup/状态导出）经同一 conn.mu 互斥。
// 旧实现读写分持不同锁（写 conn.mu、读仅 proxy.mu）导致 time.Time
// 撕裂读。在 -race 下，若无互斥本测试会报数据竞争。
func TestProxyConnLastReadConcurrentAccess(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	pc := &ProxyConn{
		ID:      "conn-race",
		ProxyID: "proxy-race",
		Conn:    conn1,
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 写侧：模拟 readFromClient 每读一次 markRead。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				pc.markRead()
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// 读侧：模拟 cleanupIdleConnections / GetProxyStatus / GetAllStatus。
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = pc.lastReadAt()
					_ = pc.lastReadAt()
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
