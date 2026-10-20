package liveness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const arpFixture = `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.50     0x1         0x2         aa:bb:cc:dd:ee:ff     *        br-lan
192.168.1.51     0x1         0x0         00:00:00:00:00:00     *        br-lan
garbage

192.168.1.52     0x1         0x2         11:22:33:44:55:66     *        wan
`

// stubSeams 注入三源 seam 并在测试结束后恢复默认实现。
type seamSet struct {
	neighbors []Neighbor
	arpErr    error
	pingErr   error
	dialErr   error
}

func stubSeams(t *testing.T, s seamSet) {
	t.Helper()
	oldRead, oldPing, oldDial := readNeighbors, pingHost, dialTCP
	readNeighbors = func() ([]Neighbor, error) { return s.neighbors, s.arpErr }
	pingHost = func(context.Context, string, time.Duration) error { return s.pingErr }
	dialTCP = func(context.Context, string, time.Duration) error { return s.dialErr }
	t.Cleanup(func() { readNeighbors, pingHost, dialTCP = oldRead, oldPing, oldDial })
}

func TestReadARPFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arp")
	if err := os.WriteFile(path, []byte(arpFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readARPFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	if got[0].IP != "192.168.1.50" || got[0].MAC != "aa:bb:cc:dd:ee:ff" ||
		got[0].State != "complete" || got[0].Dev != "br-lan" {
		t.Fatalf("bad complete entry: %+v", got[0])
	}
	if got[1].State != "incomplete" || got[1].MAC != "" {
		t.Fatalf("bad incomplete entry: %+v", got[1])
	}
	if got[2].Dev != "wan" {
		t.Fatalf("bad third entry: %+v", got[2])
	}

	if neighbors, err := readARPFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("want error for missing file, got %+v", neighbors)
	}
}

func TestProbeARP(t *testing.T) {
	cases := []struct {
		name      string
		seam      seamSet
		ip        string
		want      Verdict
		wantParts []string // Detail 必含片段
	}{
		{
			name:      "complete entry present",
			seam:      seamSet{neighbors: []Neighbor{{IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:ff", State: "complete", Dev: "br-lan"}}},
			ip:        "10.0.0.5",
			want:      VerdictPresent,
			wantParts: []string{"complete", "aa:bb:cc:dd:ee:ff", "br-lan"},
		},
		{
			name:      "incomplete entry unknown",
			seam:      seamSet{neighbors: []Neighbor{{IP: "10.0.0.5", State: "incomplete"}}},
			ip:        "10.0.0.5",
			want:      VerdictUnknown,
			wantParts: []string{"incomplete"},
		},
		{
			name:      "no entry unknown",
			seam:      seamSet{neighbors: []Neighbor{{IP: "10.0.0.9", MAC: "aa:00:00:00:00:00", State: "complete", Dev: "br-lan"}}},
			ip:        "10.0.0.5",
			want:      VerdictUnknown,
			wantParts: []string{"no neighbor entry"},
		},
		{
			name:      "read error unknown",
			seam:      seamSet{arpErr: errors.New("denied")},
			ip:        "10.0.0.5",
			want:      VerdictUnknown,
			wantParts: []string{"unavailable"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubSeams(t, tc.seam)
			got := probeARP(tc.ip)
			if got.Verdict != tc.want {
				t.Fatalf("verdict = %q, want %q (detail %q)", got.Verdict, tc.want, got.Detail)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(got.Detail, part) {
					t.Fatalf("detail %q missing %q", got.Detail, part)
				}
			}
		})
	}
}

func TestProbeICMP(t *testing.T) {
	stubSeams(t, seamSet{})
	if got := probeICMP(context.Background(), "10.0.0.5", time.Second); got.Verdict != VerdictPresent {
		t.Fatalf("verdict = %q, want present", got.Verdict)
	}
	stubSeams(t, seamSet{pingErr: errors.New("timeout")})
	got := probeICMP(context.Background(), "10.0.0.5", time.Second)
	if got.Verdict != VerdictUnknown || !strings.Contains(got.Detail, "icmp") {
		t.Fatalf("got %+v, want unknown icmp detail", got)
	}
}

func TestProbeTCP(t *testing.T) {
	// 真连接：本机临时监听端口。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	stubSeams(t, seamSet{})
	if got := probeTCP(context.Background(), "127.0.0.1", []int{int(port)}, time.Second); got.Verdict != VerdictPresent ||
		!strings.Contains(got.Detail, "connected") {
		t.Fatalf("connected case got %+v", got)
	}

	// RST：注入 ECONNREFUSED 桩（确定性），在位证据成立。
	refused := &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	stubSeams(t, seamSet{dialErr: refused})
	if got := probeTCP(context.Background(), "10.0.0.5", []int{80}, time.Second); got.Verdict != VerdictPresent ||
		!strings.Contains(got.Detail, "refused") {
		t.Fatalf("refused case got %+v", got)
	}

	// 全部不可达。
	stubSeams(t, seamSet{dialErr: errors.New("i/o timeout")})
	if got := probeTCP(context.Background(), "10.0.0.5", []int{80, 443}, time.Second); got.Verdict != VerdictUnknown ||
		!strings.Contains(got.Detail, "no evidence") {
		t.Fatalf("timeout case got %+v", got)
	}

	// 未配端口。
	if got := probeTCP(context.Background(), "10.0.0.5", nil, time.Second); got.Verdict != VerdictUnknown ||
		!strings.Contains(got.Detail, "no ports") {
		t.Fatalf("no-ports case got %+v", got)
	}
}

func TestProbe(t *testing.T) {
	stubSeams(t, seamSet{
		neighbors: []Neighbor{{IP: "10.0.0.5", MAC: "aa:00:00:00:00:01", State: "complete", Dev: "br-lan"}},
		pingErr:   errors.New("no reply"),
		dialErr:   errors.New("no route"),
	})
	before := time.Now().Unix()
	report := Probe(context.Background(), "10.0.0.5", Options{Timeout: 500 * time.Millisecond, Ports: []int{22}})
	if report.IP != "10.0.0.5" {
		t.Fatalf("ip = %q", report.IP)
	}
	if report.ARP.Verdict != VerdictPresent || report.ICMP.Verdict != VerdictUnknown || report.TCP.Verdict != VerdictUnknown {
		t.Fatalf("verdicts = %+v", report)
	}
	if report.At < before || report.At > time.Now().Unix()+1 {
		t.Fatalf("at = %d outside window", report.At)
	}

	// Timeout 缺省 → DefaultTimeout 传给各源。
	var gotTimeout time.Duration
	oldPing := pingHost
	pingHost = func(_ context.Context, _ string, d time.Duration) error { gotTimeout = d; return nil }
	t.Cleanup(func() { pingHost = oldPing })
	Probe(context.Background(), "10.0.0.5", Options{})
	if gotTimeout != DefaultTimeout {
		t.Fatalf("default timeout = %v, want %v", gotTimeout, DefaultTimeout)
	}
}

func TestIsRefused(t *testing.T) {
	if isRefused(nil) {
		t.Fatal("nil error must not be refused")
	}
	if !isRefused(fmt.Errorf("dial: %w", syscall.ECONNREFUSED)) {
		t.Fatal("wrapped ECONNREFUSED must be refused")
	}
	if !isRefused(errors.New("dial tcp 10.0.0.5:80: connection refused")) {
		t.Fatal("text refused must match")
	}
	if isRefused(errors.New("i/o timeout")) {
		t.Fatal("timeout is not refused")
	}
}

// TestDefaultClosures 直接跑三个 seam 的默认实现：ARP 读本机邻居表、ping
// 打回环、dial 连本机临时端口/打非法地址。全部容忍失败——只为覆盖默认闭包体
// （块覆盖门禁），不断言机器环境。
func TestDefaultClosures(t *testing.T) {
	_, _ = readNeighbors()                                                // 默认 /proc/net/arp
	_ = pingHost(context.Background(), "127.0.0.1", 100*time.Millisecond) // 回环 ping，失败容忍

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	if err := dialTCP(context.Background(), addr, time.Second); err != nil {
		t.Fatalf("dial local listener: %v", err)
	}
	if err := dialTCP(context.Background(), "999.999.999.999:1", time.Second); err == nil {
		t.Fatal("dial invalid addr must fail")
	}
}
