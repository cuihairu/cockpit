package liveness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// dialTCP TCP 探测注入点：默认 net.Dialer 带超时，连上即关（探测不占连接）。
var dialTCP = func(ctx context.Context, addr string, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// isRefused 连接被拒（目标回了 RST）：证明主机本身在网络层是活的，
// 只是该端口无监听（服务面问题归 healthprobe，不影响在位判定）。
func isRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	// 平台差异/注入桩错误串兜底：内核 errno 已在错误链文本里。
	return strings.Contains(err.Error(), "connection refused")
}

// probeTCP 逐端口尝试：任一端口连通 → present；任一端口被目标明确拒绝
// （RST）→ present（主机活）；全部超时/不可达 → unknown。
func probeTCP(ctx context.Context, ip string, ports []int, timeout time.Duration) Source {
	if len(ports) == 0 {
		return Source{Verdict: VerdictUnknown, Detail: "tcp: no ports configured"}
	}
	for _, port := range ports {
		addr := net.JoinHostPort(ip, strconv.Itoa(port))
		err := dialTCP(ctx, addr, timeout)
		switch {
		case err == nil:
			return Source{Verdict: VerdictPresent, Detail: fmt.Sprintf("tcp %d connected", port)}
		case isRefused(err):
			return Source{Verdict: VerdictPresent, Detail: fmt.Sprintf("tcp %d refused (host alive)", port)}
		}
	}
	return Source{Verdict: VerdictUnknown, Detail: "tcp: no evidence"}
}
