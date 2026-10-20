// Package amt 实现 OOB 统一 Provider 的 Intel AMT 探测线（vPro 终端，
// docs/review/oob-management-2026-10-20.md 落地跟踪 P0 第 3 项）。
//
// 本片边界：只做 **WS-Management Identify 免认证发现**（经 internal/oob/wsman
// 共用层，DASH 线同源）——Identify 只证「AMT 固件网络栈在位且当前可达」；
// 启用态（Provisioning）与逐能力面（power/KVM/SOL，需认证 wsman 调
// AMT_GeneralSettings 等 CIM 类）随凭据片经 vault 落位，本片多报一个能力
// 都是猜（评审红线：三态值由检测产生，不猜）。
package amt

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/wsman"
)

// Name 是本 Provider 在注册表中的名字。
const Name = "amt"

// DefaultPort 是 AMT 明文 wsman 端口（16993 TLS 随凭据片开，默认不开不猜）。
const DefaultPort = "16992"

// Options 探测器配置。
type Options struct {
	// Wsman 可注入共用 Identify 客户端配置（测试/多 Provider 复用）。
	Wsman wsman.Options
}

// Provider AMT 探测线。
type Provider struct {
	wsman *wsman.Client
}

// New 构造 AMT Provider。
func New(opts Options) *Provider {
	return &Provider{wsman: wsman.New(opts.Wsman)}
}

// Name 实现 oob.Provider。
func (p *Provider) Name() string { return Name }

// isAMT 判定 Identify 响应是否 Intel AMT：官方固件回
// "Intel(r) Active Management Technology"（老固件 "Intel AMT"）。
func isAMT(r wsman.IdentifyResponse) bool {
	n := strings.ToLower(r.ProductName)
	return strings.Contains(n, "active management technology") || strings.Contains(n, "intel amt")
}

// Probe 对目标做一次 AMT Identify 发现：
//   - 传输失败/非 wsman 端点 → 报错（不是 AMT 或不可达）；
//   - 401/403 → 端点在但无凭据，与 redfish 同口径：空能力集如实上账，不算错误；
//   - wsman 端点但产品非 AMT（DASH/通用 WBEM）→ 报错拒绝错认（操作面会打错协议）。
func (p *Provider) Probe(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
	info := oob.DeviceInfo{Provider: Name, Capabilities: map[oob.Capability]oob.State{}}

	resp, status, err := p.wsman.Identify(ctx, normalizeBase(target.Address))
	if err != nil {
		if status == 401 || status == 403 {
			return info, nil // 端点可达，凭据未接：不可判即不判
		}
		if status != 0 {
			return oob.DeviceInfo{}, fmt.Errorf("amt: %w", err)
		}
		return oob.DeviceInfo{}, fmt.Errorf("amt: %w", err)
	}
	if !isAMT(resp) {
		return oob.DeviceInfo{}, fmt.Errorf("amt: wsman endpoint is not AMT (product %q)", resp.ProductName)
	}
	return info, nil
}

// normalizeBase 补全探测基址：无 scheme 默认 http（16993/TLS 随凭据片）；
// 无 port 默认 :16992；尾斜杠去掉。带 scheme 的地址原样只去尾斜杠。
func normalizeBase(addr string) string {
	out := strings.TrimRight(addr, "/")
	if !strings.Contains(out, "://") {
		if _, _, err := net.SplitHostPort(out); err != nil {
			out += ":" + DefaultPort
		}
		out = "http://" + out
	}
	return out
}
