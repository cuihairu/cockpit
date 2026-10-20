// Package redirect 兜底转发编排的规格与校验（OpenWrt 自举节点片C，
// docs/design/oob-openwrt-bootstrap.md §3）。
//
// 本包只做纯逻辑：规则规格校验、规则命名、过期判定、ubus 参数装配。
// 真正的 ubus 调用在 internal/openwrt（客户端）与 internal/agent/rpc（RPC 面）。
//
// 三条硬纪律：
//  1. 规则名带 cockpit 前缀——本包创建与拆除的规则可被识别，清扫不会误删
//     用户自建规则，用户也不易误删本包规则（命名即归属）。
//  2. 规则必带 TTL——兜底转发是临时桥接，不得静默常驻；TTL 过期与人工拆除
//     是仅有的两条退出路径。
//  3. 过期时刻编进规则名——fw4 规则对象没有 expiry 字段，重启后进程内存清空，
//     靠名字携带过期时刻才能做到「agent 重启不丢过期账」。
//
// ubus 写契约（fw4 的 firewall.add_redirect/del_redirect）尚未经真机核实，
// 因此把参数装配隔离为纯函数：契约有出入时只需改本文件与客户端一层，
// 调用方不动。
package redirect

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RulePrefix 本包创建的规则名前缀（归属标记）。
const RulePrefix = "cockpit_"

// DefaultTTL 缺省 TTL：兜底转发默认半小时失效。
const DefaultTTL = 30 * time.Minute

// MaxTTL TTL 上限：一天。超过视为配置错误而非「常驻规则」。
const MaxTTL = 24 * time.Hour

// Spec 单条兜底转发规则的规格。
type Spec struct {
	// Label 规则可读标签（用于规则名与展示）；空则按 proto/目标推导。
	// 只允许小写字母、数字、-、_。
	Label string
	// Proto 转发协议：tcp 或 udp（icmp 无端口语义，不在此面）。
	Proto string
	// SrcZone / DstZone 防火墙区域：fw4 转发必须落在区域之间，缺证据不猜。
	SrcZone string
	DstZone string
	// SrcIP 源地址限定（空 = 不限）；DstIP 转发目标地址（必需）。
	SrcIP string
	DstIP string
	// SrcPort 源端口限定（0 = 不限）；DstPort 转发目标端口（必需，1..65535）。
	SrcPort int
	DstPort int
	// TTL 规则存活时间；<=0 用 DefaultTTL。
	TTL time.Duration
}

var labelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,40}$`)

// ttl 生效 TTL（缺省与上限收敛）。
func (s Spec) ttl() time.Duration {
	t := s.TTL
	if t <= 0 {
		t = DefaultTTL
	}
	if t > MaxTTL {
		t = MaxTTL
	}
	return t
}

// EffectiveTTL 生效 TTL（缺省与上限收敛）。
func (s Spec) EffectiveTTL() time.Duration { return s.ttl() }

// Validate 校验规格：任何一项不满足即拒绝，返回可直投给用户的错误文案。
func (s Spec) Validate() error {
	// 空标签合法：RuleName 按 proto/目标推导，避免为一条规则硬编可读名
	if s.Label != "" && !labelRe.MatchString(s.Label) {
		return fmt.Errorf("invalid label %q: want lowercase letters, digits, dot, dash or underscore, up to 41 chars", s.Label)
	}
	switch s.Proto {
	case "tcp", "udp":
	default:
		return fmt.Errorf("invalid proto %q: want tcp or udp", s.Proto)
	}
	if strings.TrimSpace(s.SrcZone) == "" {
		return fmt.Errorf("src zone is required")
	}
	if strings.TrimSpace(s.DstZone) == "" {
		return fmt.Errorf("dst zone is required")
	}
	if net.ParseIP(s.DstIP) == nil {
		return fmt.Errorf("invalid dst ip %q: want an IP address", s.DstIP)
	}
	if s.SrcIP != "" && net.ParseIP(s.SrcIP) == nil {
		return fmt.Errorf("invalid src ip %q: want an IP address", s.SrcIP)
	}
	if s.DstPort < 1 || s.DstPort > 65535 {
		return fmt.Errorf("invalid dst port %d: want 1..65535", s.DstPort)
	}
	if s.SrcPort < 0 || s.SrcPort > 65535 {
		return fmt.Errorf("invalid src port %d: want 0..65535", s.SrcPort)
	}
	if s.TTL < 0 {
		return fmt.Errorf("ttl %v must not be negative", s.TTL)
	}
	return nil
}

// RuleName 规则全名：前缀 + 标签 + 过期时刻（unix 秒）。
// 规则名确定性由「规格 + 生效时刻」唯一决定，重复下发同一规格可幂等识别。
func RuleName(s Spec, appliedAt time.Time) string {
	label := strings.TrimSpace(s.Label)
	if label == "" {
		label = s.Proto + "." + s.DstIP + "." + strconv.Itoa(s.DstPort)
	}
	return RulePrefix + label + "_" + strconv.FormatInt(appliedAt.Add(s.ttl()).Unix(), 10)
}

// SplitRuleName 反解规则名：标签与过期时刻。非本包规则名一律 ok=false。
func SplitRuleName(name string) (label string, expiry time.Time, ok bool) {
	if !Owned(name) {
		return "", time.Time{}, false
	}
	base := strings.TrimPrefix(name, RulePrefix)
	at := strings.LastIndex(base, "_")
	if at <= 0 {
		return "", time.Time{}, false
	}
	unix, err := strconv.ParseInt(base[at+1:], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	return base[:at], time.Unix(unix, 0), true
}

// Owned 判定规则名是否由本包创建（命名即归属）。
func Owned(name string) bool {
	return strings.HasPrefix(name, RulePrefix)
}

// IsExpired 判定已解析的规则名是否过期。
func IsExpired(name string, now time.Time) bool {
	_, expiry, ok := SplitRuleName(name)
	if !ok {
		return false
	}
	return !now.Before(expiry)
}

// ApplyPayload 装配 fw4 firewall.add_redirect 参数（纯装配，不发请求）。
// 字段名与 fw4 规则对象一致；端口 0 表示不限，不出现在参数里。
func ApplyPayload(s Spec, name string) map[string]interface{} {
	payload := map[string]interface{}{
		"name":      name,
		"proto":     s.Proto,
		"src":       s.SrcZone,
		"dest":      s.DstZone,
		"dest_ip":   s.DstIP,
		"dest_port": strconv.Itoa(s.DstPort),
	}
	if s.SrcIP != "" {
		payload["src_ip"] = s.SrcIP
	}
	if s.SrcPort > 0 {
		payload["src_port"] = strconv.Itoa(s.SrcPort)
	}
	return payload
}

// RemovePayload 装配 fw4 firewall.del_redirect 参数。
func RemovePayload(name string) map[string]interface{} {
	return map[string]interface{}{"name": name}
}
