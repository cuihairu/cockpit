// Package wol 实现 Wake-on-LAN 动作面：魔术包构造 + UDP 广播发送（发送注入点），
// 供带外兜底控制使用（设计 docs/design/oob-openwrt-bootstrap.md 片 A）。
// WOL 是局域网内无认证动作：发送侧不持有凭据；动作幂等（重复发送无害）。
package wol

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPort 是 WOL 约定 UDP 端口（9；部分固件也监听 7）。
	DefaultPort = 9
	// DefaultTimeout 是单次 UDP 发送超时。
	DefaultTimeout = 2 * time.Second
)

// MagicPacket 构造魔术包：6 字节 0xFF 前缀 + 目标 MAC 重复 16 次，共 102 字节。
func MagicPacket(mac string) ([]byte, error) {
	addr, err := ParseMAC(mac)
	if err != nil {
		return nil, err
	}
	pkt := make([]byte, 6+len(addr)*16)
	for i := 0; i < 6; i++ {
		pkt[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(pkt[6+i*len(addr):], addr)
	}
	return pkt, nil
}

// ParseMAC 解析 MAC 地址：冒号分隔（大小写不敏感）或无分隔符的
// 短横/点分隔形态，返回 6 字节硬件地址。
func ParseMAC(mac string) (net.HardwareAddr, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return nil, fmt.Errorf("wol: mac address is required")
	}
	if strings.Contains(mac, ":") {
		addr, err := net.ParseMAC(mac)
		if err != nil {
			return nil, fmt.Errorf("wol: invalid mac address %q", mac)
		}
		return addr, nil
	}
	raw := strings.NewReplacer("-", "", ".", "").Replace(mac)
	if len(raw) != 12 {
		return nil, fmt.Errorf("wol: invalid mac address %q", mac)
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("wol: invalid mac address %q", mac)
	}
	return b, nil
}

// SubnetBroadcast 由网段内任一 IPv4 + 前缀长度算广播地址
// （例：192.168.1.42/24 → 192.168.1.255）。前缀 > 30 无广播语义，直接报错。
func SubnetBroadcast(ipStr, prefix string) (string, error) {
	bits, err := strconv.Atoi(strings.TrimSpace(prefix))
	if err != nil || bits < 0 || bits > 30 {
		return "", fmt.Errorf("wol: invalid prefix %q (want 0..30)", prefix)
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return "", fmt.Errorf("wol: invalid ip address %q", ipStr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", fmt.Errorf("wol: broadcast is not supported for ip %q", ipStr)
	}
	mask := net.CIDRMask(bits, 32)
	b := []byte{
		ip4[0] | ^mask[0], ip4[1] | ^mask[1],
		ip4[2] | ^mask[2], ip4[3] | ^mask[3],
	}
	return net.IP(b).String(), nil
}

// Options 发送参数；Dial 是测试注入点（测试零真发 UDP 报文）。
type Options struct {
	Timeout time.Duration
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Sender 通过 UDP 发送魔术包。
type Sender struct {
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// New 创建发送器；Dial 缺省走真实 UDP 拨号（超时取 Timeout 或 DefaultTimeout）。
func New(opts Options) *Sender {
	dial := opts.Dial
	if dial == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		d := &net.Dialer{Timeout: timeout}
		dial = d.DialContext
	}
	return &Sender{dial: dial}
}

// Send 向 target（ip:port）发送一个魔术包，返回实际写入的字节数。
func (s *Sender) Send(ctx context.Context, target, mac string) (int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, fmt.Errorf("wol: target address is required")
	}
	pkt, err := MagicPacket(mac)
	if err != nil {
		return 0, err
	}
	conn, err := s.dial(ctx, "udp", target)
	if err != nil {
		return 0, fmt.Errorf("wol: dial %s: %w", target, err)
	}
	defer conn.Close()
	n, err := conn.Write(pkt)
	if err != nil {
		return 0, fmt.Errorf("wol: send to %s: %w", target, err)
	}
	return n, nil
}
