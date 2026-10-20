package rpc

// oob_provider_test.go 带外动作面测试：WOL 参数解析/目标推导/发送链路，
// 全部走注入的假 dialer、假网卡与假地址，测试不发真实 UDP 报文。

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/oob/wol"
)

// oobFakeConn 捕获写入字节，替代真实 UDP 连接。
type oobFakeConn struct {
	writes   [][]byte
	closed   bool
	writeErr error
}

func (f *oobFakeConn) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	f.writes = append(f.writes, cp)
	return len(b), nil
}

func (f *oobFakeConn) Close() error                { f.closed = true; return nil }
func (f *oobFakeConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (f *oobFakeConn) LocalAddr() net.Addr         { return &net.UDPAddr{} }
func (f *oobFakeConn) RemoteAddr() net.Addr        { return &net.UDPAddr{} }
func (f *oobFakeConn) SetDeadline(time.Time) error { return nil }
func (f *oobFakeConn) SetReadDeadline(time.Time) error {
	return nil
}
func (f *oobFakeConn) SetWriteDeadline(time.Time) error { return nil }

// fakeSender 记录 dial 目标与写入字节，替代真实 UDP 发送器。
type fakeSender struct {
	sender  *wol.Sender
	conn    *oobFakeConn
	targets []string
}

func newFakeSender(t *testing.T) *fakeSender {
	t.Helper()
	f := &fakeSender{conn: &oobFakeConn{}}
	f.sender = wol.New(wol.Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		f.targets = append(f.targets, network+" "+addr)
		return f.conn, nil
	}})
	return f
}

// oobIface 构造测试网卡（Go 1.27 起 net.Interface 为 struct，可直接构造）。
func oobIface(name string, flags net.Flags) net.Interface {
	return net.Interface{Index: 1, Name: name, MTU: 1500, Flags: flags}
}

// oobCIDRs 把 CIDR 字面量转成地址列表（IP 取网段内主机地址）。
func oobCIDRs(cidrs ...string) []net.Addr {
	out := make([]net.Addr, 0, len(cidrs))
	for _, c := range cidrs {
		ip, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		ipNet.IP = ip
		out = append(out, ipNet)
	}
	return out
}

// oobIfaceSeed 按网卡名注入的地址种子。
type oobIfaceSeed struct {
	addrs []string // CIDR 字面量（构造成 *net.IPNet）
	raw   []string // 原始地址（*net.IPAddr，非 IPNet 形态）
	err   error
}

// stubIfaceSeam 替换网卡枚举与地址读取注入点，并在测试结束恢复。
func stubIfaceSeam(t *testing.T, ifaces []net.Interface, seeds map[string]oobIfaceSeed) {
	t.Helper()
	origIface, origAddrs := netInterfaces, ifaceAddrs
	t.Cleanup(func() { netInterfaces, ifaceAddrs = origIface, origAddrs })
	netInterfaces = func() ([]net.Interface, error) { return ifaces, nil }
	ifaceAddrs = func(ifc net.Interface) ([]net.Addr, error) {
		s, ok := seeds[ifc.Name]
		if !ok {
			return nil, errors.New("no such interface")
		}
		if s.err != nil {
			return nil, s.err
		}
		out := oobCIDRs(s.addrs...)
		for _, ipStr := range s.raw {
			out = append(out, &net.IPAddr{IP: net.ParseIP(ipStr)})
		}
		return out, nil
	}
}

func TestOobProviderType(t *testing.T) {
	stubIfaceSeam(t, nil, nil)
	if p := NewOobProvider(newFakeSender(t).sender, nil); p.Type() != "oob" {
		t.Fatal("Type must be oob")
	}
}

func TestOobUnknownAction(t *testing.T) {
	stubIfaceSeam(t, nil, nil)
	p := NewOobProvider(newFakeSender(t).sender, nil)
	if _, err := p.Call("wake", map[string]interface{}{"mac": "00:11:22:aa:bb:cc"}); err == nil ||
		!strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("unknown action: %v", err)
	}
}

// TestOobWolExplicitTarget 显式 ip:port 原样透传，不触碰本机网卡枚举。
func TestOobWolExplicitTarget(t *testing.T) {
	orig := netInterfaces
	var called bool
	t.Cleanup(func() { netInterfaces = orig })
	netInterfaces = func() ([]net.Interface, error) {
		called = true
		return nil, errors.New("must not be called")
	}
	f := newFakeSender(t)
	p := NewOobProvider(f.sender, nil)
	out, err := p.Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "target": "255.255.255.255:9",
	})
	if err != nil {
		t.Fatalf("wol: %v", err)
	}
	if called {
		t.Fatal("explicit target must not enumerate interfaces")
	}
	if len(f.targets) != 1 || f.targets[0] != "udp 255.255.255.255:9" {
		t.Fatalf("targets = %v", f.targets)
	}
	want, _ := wol.MagicPacket("00:11:22:aa:bb:cc")
	if len(f.conn.writes) != 1 || string(f.conn.writes[0]) != string(want) {
		t.Fatal("payload must be the magic packet")
	}
	res, ok := out.(map[string]interface{})
	if !ok || res["sent"] != 102 || res["target"] != "255.255.255.255:9" || res["mac"] != "00:11:22:aa:bb:cc" {
		t.Fatalf("result = %v", out)
	}
	if !f.conn.closed {
		t.Fatal("conn must be closed")
	}
}

// TestOobWolExplicitIPOtherPort 纯 IP + 自定义端口拼成 ip:port（JSON 数字与 int 两种端口形态）。
func TestOobWolExplicitIPOtherPort(t *testing.T) {
	stubIfaceSeam(t, nil, nil)
	f := newFakeSender(t)
	if _, err := NewOobProvider(f.sender, nil).Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "target": "192.168.1.255", "port": float64(7),
	}); err != nil {
		t.Fatalf("wol: %v", err)
	}
	if f.targets[0] != "udp 192.168.1.255:7" {
		t.Fatalf("target = %v", f.targets)
	}
	if _, err := NewOobProvider(f.sender, nil).Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "target": "192.168.1.255", "port": int(8),
	}); err != nil {
		t.Fatalf("wol: %v", err)
	}
	if f.targets[1] != "udp 192.168.1.255:8" {
		t.Fatalf("target = %v", f.targets)
	}
}

// TestOobWolDerivedBroadcast 无 target：按首个可广播非回环 IPv4 网段算广播地址。
func TestOobWolDerivedBroadcast(t *testing.T) {
	stubIfaceSeam(t, []net.Interface{
		oobIface("lo", net.FlagLoopback|net.FlagUp),
		oobIface("br-down", net.FlagLoopback),
		oobIface("br-ipv6", net.FlagUp),
		oobIface("br-lan", net.FlagUp|net.FlagBroadcast),
	}, map[string]oobIfaceSeed{
		"br-down": {addrs: []string{"192.168.2.1/24"}}, // 未 up，跳过
		"br-ipv6": {addrs: []string{"2001:db8::1/64"}}, // 仅 IPv6，跳过
		"br-lan":  {addrs: []string{"192.168.1.1/24"}},
	})
	f := newFakeSender(t)
	out, err := NewOobProvider(f.sender, nil).Call("wol", map[string]interface{}{"mac": "00:11:22:aa:bb:cc"})
	if err != nil {
		t.Fatalf("wol: %v", err)
	}
	if f.targets[0] != "udp 192.168.1.255:9" {
		t.Fatalf("target = %v", f.targets)
	}
	if out.(map[string]interface{})["target"] != "192.168.1.255:9" {
		t.Fatalf("result = %v", out)
	}
}

// TestOobWolIfaceFilter 指定网卡时只从该网卡网段推导。
func TestOobWolIfaceFilter(t *testing.T) {
	stubIfaceSeam(t, []net.Interface{
		oobIface("br-lan", net.FlagUp|net.FlagBroadcast),
		oobIface("br-vpn", net.FlagUp|net.FlagBroadcast),
	}, map[string]oobIfaceSeed{
		"br-lan": {addrs: []string{"192.168.1.1/24"}},
		"br-vpn": {addrs: []string{"10.10.0.1/24"}},
	})
	f := newFakeSender(t)
	if _, err := NewOobProvider(f.sender, nil).Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "iface": "br-vpn", "port": "7",
	}); err != nil {
		t.Fatalf("wol: %v", err)
	}
	if f.targets[0] != "udp 10.10.0.255:7" {
		t.Fatalf("target = %v", f.targets)
	}
}

// TestOobWolNoBroadcastable 无可广播网卡时报错（不发送）。
func TestOobWolNoBroadcastable(t *testing.T) {
	stubIfaceSeam(t, []net.Interface{
		oobIface("lo", net.FlagLoopback|net.FlagUp),
		oobIface("br-p2p", net.FlagUp|net.FlagBroadcast),
		oobIface("br-empty", net.FlagUp|net.FlagBroadcast),
		oobIface("br-err", net.FlagUp|net.FlagBroadcast),
		oobIface("br-raw", net.FlagUp|net.FlagBroadcast),
	}, map[string]oobIfaceSeed{
		"lo":       {addrs: []string{"127.0.0.1/8"}},
		"br-p2p":   {addrs: []string{"10.0.0.1/31"}}, // 前缀 > 30 无广播语义
		"br-empty": {},                               // 无地址
		"br-err":   {err: errors.New("boom addrs")},  // 地址读取失败
		"br-raw":   {raw: []string{"10.1.2.3"}},      // 非 IPNet 地址
	})
	f := newFakeSender(t)
	if _, err := NewOobProvider(f.sender, nil).Call("wol", map[string]interface{}{"mac": "00:11:22:aa:bb:cc"}); err == nil ||
		!strings.Contains(err.Error(), "no broadcastable") {
		t.Fatalf("want no-broadcastable error, got %v", err)
	}
	if len(f.targets) != 0 {
		t.Fatalf("must not dial, got %v", f.targets)
	}
}

// TestOobWolParamErrors 参数与目标校验的错误分支。
func TestOobWolParamErrors(t *testing.T) {
	stubIfaceSeam(t, []net.Interface{oobIface("br-lan", net.FlagUp|net.FlagBroadcast)}, map[string]oobIfaceSeed{
		"br-lan": {addrs: []string{"192.168.1.1/24"}},
	})
	f := newFakeSender(t)
	p := NewOobProvider(f.sender, nil)
	cases := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{"nil params", nil, "params required"},
		{"missing mac", map[string]interface{}{"target": "255.255.255.255:9"}, "mac required"},
		{"blank mac", map[string]interface{}{"mac": "  "}, "mac required"},
		{"bad mac", map[string]interface{}{"mac": "zz", "target": "255.255.255.255:9"}, "invalid mac"},
		{"bad target host", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "target": "bad:9"}, "invalid target host"},
		{"bad target", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "target": "not-an-ip"}, "invalid target"},
		{"bad port", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "port": "abc"}, "invalid port"},
		{"port too high", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "port": float64(65536)}, "invalid port"},
		{"port negative", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "port": int(-1)}, "invalid port"},
		{"port fraction", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "port": float64(1.5)}, "invalid port"},
		{"port bad type", map[string]interface{}{"mac": "00:11:22:aa:bb:cc", "port": []int{1}}, "invalid port"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := p.Call("wol", c.params); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
	if len(f.targets) != 0 {
		t.Fatalf("invalid params must not dial, got %v", f.targets)
	}
}

// TestOobWolInterfaceAndSendErrors 网卡枚举失败与发送失败的透传。
func TestOobWolInterfaceAndSendErrors(t *testing.T) {
	orig := netInterfaces
	t.Cleanup(func() { netInterfaces = orig })
	netInterfaces = func() ([]net.Interface, error) { return nil, errors.New("boom ifaces") }
	if _, err := NewOobProvider(newFakeSender(t).sender, nil).Call("wol", map[string]interface{}{"mac": "00:11:22:aa:bb:cc"}); err == nil ||
		!strings.Contains(err.Error(), "list interfaces") {
		t.Fatalf("iface error: %v", err)
	}

	dialFail := wol.New(wol.Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, errors.New("boom dial")
	}})
	stubIfaceSeam(t, nil, nil)
	if _, err := NewOobProvider(dialFail, nil).Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "target": "255.255.255.255:9",
	}); err == nil || !strings.Contains(err.Error(), "dial") {
		t.Fatalf("dial error: %v", err)
	}

	writeFail := wol.New(wol.Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return &oobFakeConn{writeErr: errors.New("boom write")}, nil
	}})
	if _, err := NewOobProvider(writeFail, nil).Call("wol", map[string]interface{}{
		"mac": "00:11:22:aa:bb:cc", "target": "255.255.255.255:9",
	}); err == nil || !strings.Contains(err.Error(), "send to") {
		t.Fatalf("write error: %v", err)
	}
}

// TestNewOobProviderDefaults 零参构造走生产默认值：网卡枚举与地址读取都用
// 注入替身（全部网卡地址为回环，派生不出广播地址），全程不发真实 UDP 报文。
func TestNewOobProviderDefaults(t *testing.T) {
	origIface, origAddrs := netInterfaces, ifaceAddrs
	t.Cleanup(func() { netInterfaces, ifaceAddrs = origIface, origAddrs })
	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			oobIface("lo", net.FlagLoopback|net.FlagUp),
			oobIface("br-lan", net.FlagUp|net.FlagBroadcast),
		}, nil
	}
	ifaceAddrs = func(net.Interface) ([]net.Addr, error) {
		return oobCIDRs("127.0.0.1/8"), nil
	}
	p := NewOobProvider(nil, nil)
	if _, err := p.Call("wol", map[string]interface{}{"mac": "00:11:22:aa:bb:cc"}); err == nil ||
		!strings.Contains(err.Error(), "no broadcastable") {
		t.Fatalf("want no-broadcastable error, got %v", err)
	}
}

// TestIfaceSeamDefaults 注入点默认值即生产取值：只读本机接口列表与地址
// （无网络 I/O），未注入桩时行为与 net.Interfaces/net.Interface.Addrs 一致。
func TestIfaceSeamDefaults(t *testing.T) {
	ifaces, err := netInterfaces()
	if err != nil {
		t.Fatalf("default netInterfaces: %v", err)
	}
	var faces, addrsCount int
	for _, ifc := range ifaces {
		addrs, err := ifaceAddrs(ifc)
		if err != nil {
			continue
		}
		faces++
		addrsCount += len(addrs)
	}
	if faces == 0 || addrsCount == 0 {
		t.Fatalf("default seams read no addresses (faces=%d addrs=%d)", faces, addrsCount)
	}
}
