// Package redfish 实现 OOB 统一 Provider 的 Redfish 探测线（服务器线，
// docs/review/oob-management-2026-10-20.md 落地跟踪 P0 第 2 项，基座
// docs/design/bmc-capabilities.md）。
//
// 本片边界：只做**只读发现探测**（service root + System/Manager/Bios 资源核实），
// 用标准库 HTTP 客户端 + httptest 契约测试锁语义；协议栈（gofish/bmclib）与
// 凭据面（vault 引用 → Basic/Session 认证）、电源等操作面随 bmc-capabilities
// §7 的 BMC-B3/B4 批次落位，不在本片自研 quirk。
package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/oob"
)

// Name 是本 Provider 在注册表中的名字。
const Name = "redfish"

// probeTimeout 沿 bmc-capabilities §1 双栈策略的 service root 探测超时口径。
const probeTimeout = 2 * time.Second

// Options 探测器配置。
type Options struct {
	// Client 可注入（测试用 httptest 服务器走 http）；nil 时用默认客户端。
	Client *http.Client
}

// Provider Redfish 探测线。
type Provider struct {
	client *http.Client
}

// New 构造 Redfish Provider。
func New(opts Options) *Provider {
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: probeTimeout}
	}
	return &Provider{client: opts.Client}
}

// Name 实现 oob.Provider。
func (p *Provider) Name() string { return Name }

// refLink 是 Redfish @odata.id 引用。
type refLink struct {
	OdataID string `json:"@odata.id"`
}

// serviceRoot 只取本片用到的字段（多余字段忽略，前向兼容）。
type serviceRoot struct {
	RedfishVersion string  `json:"RedfishVersion"`
	Product        string  `json:"Product"`
	Systems        refLink `json:"Systems"`
	Managers       refLink `json:"Managers"`
}

type collection struct {
	Members []refLink `json:"Members"`
}

// computerSystem 只读面：C1 电源状态 + C2 Reset 动作存在性 + Bios/Boot 链接。
type computerSystem struct {
	PowerState string `json:"PowerState"`
	Actions    struct {
		Reset json.RawMessage `json:"#ComputerSystem.Reset"`
	} `json:"Actions"`
	Bios refLink         `json:"Bios"`
	Boot json.RawMessage `json:"Boot"`
}

// manager 只取 C6 硬件清单面确认资源在位（字段留档，quirk 片再投影）。
type manager struct {
	Model           string `json:"Model"`
	FirmwareVersion string `json:"FirmwareVersion"`
}

// Probe 对目标做一次 Redfish 只读发现：
//   - 传输失败/非 2xx/缺 RedfishVersion → 报错（不是 Redfish 服务或不可达）；
//   - 401/403 → 服务在但凭据面未接，三态不可判就不判：空能力集如实上账，不算错误；
//   - 其余子资源失败只降对应能力的 reachable，不炸探测（逐资源降级，
//     service root 的门槛不放宽）。
//
// 能力三态全部由响应字段产生，缺证据的能力不猜（评审红线：厂商宣称不直填）。
func (p *Provider) Probe(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
	origin := target.Address
	if !strings.Contains(origin, "://") {
		origin = "https://" + origin // 测试注入走 http；生产 BMC 默认 HTTPS（§5.2 TLS 校验）
	}
	origin = strings.TrimRight(origin, "/")

	info := oob.DeviceInfo{Provider: Name, Capabilities: map[oob.Capability]oob.State{}}

	var root serviceRoot
	status, err := p.getJSON(ctx, origin+"/redfish/v1", &root)
	if err != nil {
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return info, nil // 服务可达，凭据未接：不可判即不判
		}
		if status != 0 {
			return oob.DeviceInfo{}, fmt.Errorf("redfish: not a redfish service (root status %d)", status)
		}
		return oob.DeviceInfo{}, fmt.Errorf("redfish: %w", err)
	}
	if root.RedfishVersion == "" {
		return oob.DeviceInfo{}, fmt.Errorf("redfish: service root lacks RedfishVersion, not a redfish service")
	}

	// Systems → 计算系统：power/bios/boot 的证据面
	if link := root.Systems.OdataID; link != "" {
		var sys computerSystem
		if sysURL, ok := p.firstMember(ctx, origin, link); ok {
			if ok2 := p.getOK(ctx, sysURL, &sys); ok2 {
				if sys.Actions.Reset != nil {
					// C2 动作面在：supported+enabled，reachable 由本次取到证实
					info.Capabilities[oob.CapabilityPower] = oob.State{Supported: true, Enabled: true, Reachable: true}
				}
				if sys.Bios.OdataID != "" {
					// BIOS 配置面暴露即视为 enabled；reachable 以取到资源为准
					var bios struct{}
					info.Capabilities[oob.CapabilityBIOS] = oob.State{
						Supported: true, Enabled: true, Reachable: p.getOK(ctx, p.abs(origin, sys.Bios.OdataID), &bios),
					}
				}
				if sys.Boot != nil {
					info.Capabilities[oob.CapabilityBoot] = oob.State{Supported: true, Enabled: true, Reachable: true}
				}
			}
		}
	}

	// Managers → BMC 清单（C6）：inventory 证据面；reachable 以成员资源取到为准
	if link := root.Managers.OdataID; link != "" {
		var mgr manager
		if mgrURL, ok := p.firstMember(ctx, origin, link); ok && p.getOK(ctx, mgrURL, &mgr) {
			info.Capabilities[oob.CapabilityInventory] = oob.State{Supported: true, Enabled: true, Reachable: true}
		} else {
			info.Capabilities[oob.CapabilityInventory] = oob.State{Supported: true, Enabled: true}
		}
	}

	// console/recovery 是 OEM 面（bmc-capabilities C7/C8），标准探测无证据不猜。
	return info, nil
}

// getJSON GET 一个 Redfish 资源并解码进 into。status==0 表示传输层失败；
// 非 2xx 返回状态码与错误；2xx 但 body 非法 JSON 返回状态码与解码错误。
func (p *Provider) getJSON(ctx context.Context, url string, into any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return resp.StatusCode, fmt.Errorf("invalid JSON body: %w", err)
	}
	return resp.StatusCode, nil
}

// getOK 取资源，只回答「是否合法取到」（子资源降级判定用，错误细节丢弃）。
func (p *Provider) getOK(ctx context.Context, url string, into any) bool {
	code, err := p.getJSON(ctx, url, into)
	return err == nil && code >= 200 && code < 300
}

// firstMember 解析集合并返回第一个成员的绝对 URL；集合缺失/空/成员取不到均
// 返回 ok=false（该集合对应的能力降级，不报错）。
func (p *Provider) firstMember(ctx context.Context, origin, collectionLink string) (string, bool) {
	var col collection
	if !p.getOK(ctx, p.abs(origin, collectionLink), &col) {
		return "", false
	}
	if len(col.Members) == 0 {
		return "", false // 集合合法但无成员——同样不猜能力
	}
	return p.abs(origin, col.Members[0].OdataID), true
}

// abs 把资源内 @odata.id（根起绝对路径，如 /redfish/v1/Systems）拼成完整 URL。
func (p *Provider) abs(origin, id string) string {
	if strings.Contains(id, "://") {
		return id
	}
	return origin + "/" + strings.TrimLeft(id, "/")
}
