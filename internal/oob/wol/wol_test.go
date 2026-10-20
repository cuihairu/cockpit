package wol

// wol_test.go 魔术包构造、MAC 解析、网段广播计算与 UDP 发送（conn 注入，
// 测试零真发报文）。

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeConn 记录写入的字节，替代真实 UDP 连接。
type fakeConn struct {
	writes   [][]byte
	closed   bool
	writeErr error
}

func (f *fakeConn) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	f.writes = append(f.writes, cp)
	return len(b), nil
}

func (f *fakeConn) Close() error                     { f.closed = true; return nil }
func (f *fakeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (f *fakeConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (f *fakeConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestMagicPacket(t *testing.T) {
	pkt, err := MagicPacket("00:11:22:aa:bb:cc")
	if err != nil {
		t.Fatalf("MagicPacket: %v", err)
	}
	if len(pkt) != 102 {
		t.Fatalf("packet length = %d, want 102", len(pkt))
	}
	want := make([]byte, 102)
	for i := 0; i < 6; i++ {
		want[i] = 0xFF
	}
	mac := []byte{0x00, 0x11, 0x22, 0xaa, 0xbb, 0xcc}
	for i := 0; i < 16; i++ {
		copy(want[6+i*6:], mac)
	}
	for i := range pkt {
		if pkt[i] != want[i] {
			t.Fatalf("byte %d = 0x%02x, want 0x%02x", i, pkt[i], want[i])
		}
	}
}

func TestParseMAC(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		errHas string
	}{
		{"00:11:22:aa:bb:cc", "00:11:22:aa:bb:cc", ""},
		{"00:11:22:AA:BB:CC", "00:11:22:aa:bb:cc", ""},
		{"00-11-22-aa-bb-cc", "00:11:22:aa:bb:cc", ""},
		{"00.11.22.aa.bb.cc", "00:11:22:aa:bb:cc", ""},
		{"001122aabbcc", "00:11:22:aa:bb:cc", ""},
		{" 00:11:22:aa:bb:cc ", "00:11:22:aa:bb:cc", ""},
		{"", "", "required"},
		{"00:11:22:aa:bb", "", "invalid"},
		{"00:11:22:zz:bb:cc", "", "invalid"},
		{"001122aabb", "", "invalid"},
		{"001122aabbccd", "", "invalid"},
		{"00-11-22-gg-bb-cc", "", "invalid"},
	}
	for _, c := range cases {
		got, err := ParseMAC(c.in)
		if c.errHas != "" {
			if err == nil || !strings.Contains(err.Error(), c.errHas) {
				t.Errorf("ParseMAC(%q) = %v, want error containing %q", c.in, err, c.errHas)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMAC(%q) error: %v", c.in, err)
			continue
		}
		if !strings.EqualFold(got.String(), c.want) {
			t.Errorf("ParseMAC(%q) = %s, want %s", c.in, got.String(), c.want)
		}
	}
}

func TestSubnetBroadcast(t *testing.T) {
	cases := []struct {
		ip, prefix string
		want       string
		errHas     string
	}{
		{"192.168.1.42", "24", "192.168.1.255", ""},
		{"10.0.0.5", "8", "10.255.255.255", ""},
		{"172.16.20.7", "16", "172.16.255.255", ""},
		{"192.168.1.128", "25", "192.168.1.255", ""},
		{"10.0.0.0", "0", "255.255.255.255", ""},
		{"10.0.0.0", "31", "", "invalid prefix"},
		{"10.0.0.0", "32", "", "invalid prefix"},
		{"10.0.0.0", "-1", "", "invalid prefix"},
		{"10.0.0.0", "xx", "", "invalid prefix"},
		{"not-an-ip", "24", "", "invalid ip address"},
		{"fd00::1", "8", "", "not supported"},
	}
	for _, c := range cases {
		got, err := SubnetBroadcast(c.ip, c.prefix)
		if c.errHas != "" {
			if err == nil || !strings.Contains(err.Error(), c.errHas) {
				t.Errorf("SubnetBroadcast(%s,%s) = %q, %v; want error containing %q", c.ip, c.prefix, got, err, c.errHas)
			}
			continue
		}
		if err != nil {
			t.Errorf("SubnetBroadcast(%s,%s) error: %v", c.ip, c.prefix, err)
			continue
		}
		if got != c.want {
			t.Errorf("SubnetBroadcast(%s,%s) = %s, want %s", c.ip, c.prefix, got, c.want)
		}
	}
}

// TestSend 发送路径：conn 收到的字节就是魔术包，目标地址透传。
func TestSend(t *testing.T) {
	conn := &fakeConn{}
	var gotNet, gotAddr string
	s := New(Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		gotNet, gotAddr = network, addr
		return conn, nil
	}})
	n, err := s.Send(context.Background(), "255.255.255.255:9", "00:11:22:aa:bb:cc")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if n != 102 || len(conn.writes) != 1 || len(conn.writes[0]) != 102 {
		t.Fatalf("sent %d bytes in %d writes", n, len(conn.writes))
	}
	want, _ := MagicPacket("00:11:22:aa:bb:cc")
	if string(conn.writes[0]) != string(want) {
		t.Fatal("payload differs from magic packet")
	}
	if gotNet != "udp" || gotAddr != "255.255.255.255:9" {
		t.Fatalf("dial(%q, %q)", gotNet, gotAddr)
	}
	if !conn.closed {
		t.Fatal("conn must be closed after send")
	}
}

func TestSendErrors(t *testing.T) {
	s := New(Options{})

	if _, err := s.Send(context.Background(), "", "00:11:22:aa:bb:cc"); err == nil ||
		!strings.Contains(err.Error(), "target address is required") {
		t.Fatalf("empty target: %v", err)
	}
	if _, err := s.Send(context.Background(), "   ", "00:11:22:aa:bb:cc"); err == nil {
		t.Fatalf("blank target: %v", err)
	}
	if _, err := s.Send(context.Background(), "255.255.255.255:9", "zz"); err == nil ||
		!strings.Contains(err.Error(), "invalid mac address") {
		t.Fatalf("bad mac: %v", err)
	}

	dialFail := New(Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, errors.New("boom dial")
	}})
	if _, err := dialFail.Send(context.Background(), "255.255.255.255:9", "00:11:22:aa:bb:cc"); err == nil ||
		!strings.Contains(err.Error(), "dial") {
		t.Fatalf("dial failure: %v", err)
	}

	writeFail := New(Options{Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return &fakeConn{writeErr: errors.New("boom write")}, nil
	}})
	if _, err := writeFail.Send(context.Background(), "255.255.255.255:9", "00:11:22:aa:bb:cc"); err == nil ||
		!strings.Contains(err.Error(), "send to") {
		t.Fatalf("write failure: %v", err)
	}
}
