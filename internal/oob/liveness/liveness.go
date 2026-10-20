// Package liveness 实现带外探活层（OpenWrt 自举节点设计 片B）：
// 对目标做 ARP/ICMP/TCP 三源独立探测，判定输出「在位/不在位/未知」三态
// 而非布尔（与 OOB 三态同纪律：探测不到 ≠ 不存在）。三源独立失败互不影响。
// 语义边界：本包只回答「网络层在位」，不宣称服务健康（服务面归 healthprobe）。
package liveness

import (
	"context"
	"time"
)

// DefaultTimeout 单源探测缺省超时。
const DefaultTimeout = 2 * time.Second

// Verdict 三态判定。
type Verdict string

const (
	// VerdictUnknown 未知：探测失败或无证据（探测不到 ≠ 不存在）。
	VerdictUnknown Verdict = "unknown"
	// VerdictPresent 在位：有明确的在位证据（ARP 邻居条目、ICMP 应答、
	// TCP 连通或被目标机明确拒绝）。
	VerdictPresent Verdict = "present"
	// VerdictAbsent 不在位：有显式不在位证据。当前三源不产出该态
	// （探测不到 ≠ 不存在），保留给未来带主动复核的证据源。
	VerdictAbsent Verdict = "absent"
)

// Source 单源判定结果（来源可溯，Detail 只记证据描述不落敏感信息）。
type Source struct {
	Verdict Verdict `json:"verdict"`
	Detail  string  `json:"detail,omitempty"`
}

// Report 单目标三源判定。
type Report struct {
	IP   string `json:"ip"`
	ARP  Source `json:"arp"`
	ICMP Source `json:"icmp"`
	TCP  Source `json:"tcp"`
	At   int64  `json:"at"` // 判定时刻（unix 秒）
}

// Options 探测参数：Ports 是 TCP 源的探测端口清单（空则 TCP 源报 unknown）。
type Options struct {
	Timeout time.Duration
	Ports   []int
}

// Probe 对单目标跑三源独立探测：任一源失败不影响其他源，各源自带三态。
func Probe(ctx context.Context, ip string, opts Options) Report {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Report{
		IP:   ip,
		ARP:  probeARP(ip),
		ICMP: probeICMP(ctx, ip, timeout),
		TCP:  probeTCP(ctx, ip, opts.Ports, timeout),
		At:   time.Now().Unix(),
	}
}
