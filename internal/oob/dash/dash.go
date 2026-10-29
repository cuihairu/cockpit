// Package dash 实现 OOB 统一 Provider 的 AMD DASH 探测线（Ryzen PRO 终端，
// docs/review/oob-management-2026-10-20.md 落地跟踪 P0 第 4 项）。
//
// 本片边界：只做 **WS-Management Identify 免认证发现**（经 internal/oob/wsman
// 共用层，AMT 线同源）——Identify 只证「DASH 固件网络栈在位且当前可达」；
// 启用态与逐能力面（power/console/KVM，需认证 wsman 调 DASH CIM 类）随凭据片
// 经 vault 落位，本片多报一个能力都是猜（评审红线：三态值由检测产生，不猜）。
package dash

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/wsman"
)

// Name 是本 Provider 在注册表中的名字。
const Name = "dash"

// DefaultPort 是 DASH 明文 wsman 端口（16993 TLS 随凭据片开，默认不开不猜；
// OpenWrt 兜底转发的 AMT/DASH 一组同用 16992/16993）。
const DefaultPort = "16992"

// Options 探测器配置。
type Options struct {
	// Wsman 可注入共用 Identify 客户端配置（测试/多 Provider 复用）。
	Wsman wsman.Options
}

// Provider DASH 探测线。
type Provider struct {
	wsman *wsman.Client
}

// New 构造 DASH Provider。
func New(opts Options) *Provider {
	return &Provider{wsman: wsman.New(opts.Wsman)}
}

// Name 实现 oob.Provider。
func (p *Provider) Name() string { return Name }

// isDASH 判定 Identify 响应是否 AMD DASH：官方固件 ProductName 回
// "DASH"/"AMD DASH CIM" 类（大小写与厂商缀漂移，统一小写匹配）。
func isDASH(r wsman.IdentifyResponse) bool {
	return strings.Contains(strings.ToLower(r.ProductName), "dash")
}

// Probe 对目标做一次 DASH Identify 发现：
//   - 传输失败/非 wsman 端点 → 报错（不是 DASH 或不可达）；
//   - 401/403 → 端点在但无凭据，与 AMT/redfish 同口径：空能力集如实上账，不算错误；
//   - wsman 端点但产品非 DASH（AMT/通用 WBEM）→ 报错拒绝错认（操作面会打错协议）。
func (p *Provider) Probe(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
	info := oob.DeviceInfo{Provider: Name, Capabilities: map[oob.Capability]oob.State{}}

	resp, status, err := p.wsman.Identify(ctx, normalizeBase(target.Address))
	if err != nil {
		if status == 401 || status == 403 {
			return info, nil // 端点可达，凭据未接：不可判即不判
		}
		return oob.DeviceInfo{}, fmt.Errorf("dash: %w", err)
	}
	if !isDASH(resp) {
		return oob.DeviceInfo{}, fmt.Errorf("dash: wsman endpoint is not DASH (product %q)", resp.ProductName)
	}
	info.Discovered = true
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
