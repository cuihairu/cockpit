// Package healthprobe 探活原语（agent-core ① 通用能力层）：HTTP/TCP 探针
// Checker。只回答「这一次探活成没成」——阈值状态机/自愈策略/事件环留在
// 调用方（core 不拥业务策略）。systemd 等平台型探针不在此（平台差异归
// core/platform，执行原语归调用方）。
package healthprobe

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
)

// bodyDrain http 探针读完少量 body 再关（连接复用 + 防 RST 噪音）
const bodyDrain = 4 << 10

// Checker 一次探活执行：err 非 nil 即失败（原因由调用方摘述）
type Checker interface {
	Check(ctx context.Context) error
}

// HTTPChecker http 探针：GET target，状态码精确等于 expectStatus（僵死探测
// 的关键——进程活着但隧道死时非 200）
type HTTPChecker struct {
	URL          string
	ExpectStatus int          // 0 视为 200
	Client       *http.Client // nil 用 http.DefaultClient（测试/装配注入点）
}

// Check 执行一次 http 探活
func (c HTTPChecker) Check(ctx context.Context) error {
	if c.ExpectStatus == 0 {
		c.ExpectStatus = http.StatusOK
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, bodyDrain))
	if resp.StatusCode != c.ExpectStatus {
		return fmt.Errorf("HTTP %d (expect %d)", resp.StatusCode, c.ExpectStatus)
	}
	return nil
}

// TCPChecker tcp 探针：DialContext 连通即活（「进程监听还在」类）
type TCPChecker struct {
	Addr   string
	Dialer *net.Dialer // nil 每次用零值（同既有每次新建 Dialer 语义）
}

// Check 执行一次 tcp 探活
func (c TCPChecker) Check(ctx context.Context) error {
	d := c.Dialer
	if d == nil {
		d = &net.Dialer{}
	}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return err
	}
	return conn.Close()
}
