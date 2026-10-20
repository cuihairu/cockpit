package agent

import (
	"context"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/oob/liveness"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestParseLivenessTargets 目标清单解析：合法 IP（含 IPv6）/空串/非法项报错。
func TestParseLivenessTargets(t *testing.T) {
	got, err := parseLivenessTargets(" 10.0.0.5, 10.0.0.6 , 2001:db8::1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].IP != "10.0.0.5" || got[2].IP != "2001:db8::1" {
		t.Fatalf("targets = %+v", got)
	}
	if got[0].Ports != nil {
		t.Fatalf("per-target ports must stay empty: %+v", got[0])
	}

	empty, err := parseLivenessTargets(" , ")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("blank input = %+v", empty)
	}

	for _, bad := range []string{"10.0.0.5,not-an-ip", "10.0.0.5,999.999.1.1"} {
		if _, err := parseLivenessTargets(bad); err == nil {
			t.Fatalf("want error for %q", bad)
		}
	}
}

// TestParseLivenessPorts 端口清单解析：合法/空/越界/非数字。
func TestParseLivenessPorts(t *testing.T) {
	got, err := parseLivenessPorts(" 22, 80 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 22 || got[1] != 80 {
		t.Fatalf("ports = %v", got)
	}

	empty, err := parseLivenessPorts("")
	if err != nil || len(empty) != 0 {
		t.Fatalf("blank input = %v, err=%v", empty, err)
	}

	for _, bad := range []string{"0", "65536", "abc", "-1", "1.5"} {
		if _, err := parseLivenessPorts(bad); err == nil {
			t.Fatalf("want error for %q", bad)
		}
	}
}

// TestNewLivenessScheduler 装配：无目标清单返回 nil；全局端口回填给未自带端口的
// 目标；自带端口的目标优先；周期按秒配置；OnCycle 推送一条 liveness_report。
func TestNewLivenessScheduler(t *testing.T) {
	cfg := Config{LivenessTargets: []liveness.Target{{IP: "10.0.0.5", Ports: []int{443}}, {IP: "10.0.0.6"}}}
	if got := newLivenessScheduler(Config{}, nil); got != nil {
		t.Fatalf("no targets = %+v", got)
	}

	var sent []*protocol.Message
	send := func(m *protocol.Message) error { sent = append(sent, m); return nil }
	cfg.LivenessPorts = []int{22, 80}
	cfg.LivenessIntervalSeconds = 7
	s := newLivenessScheduler(cfg, send)
	if s == nil {
		t.Fatal("scheduler must be built")
	}
	if s.Interval != 7*time.Second {
		t.Fatalf("interval = %v", s.Interval)
	}
	if s.Targets[0].Ports[0] != 443 || len(s.Targets[0].Ports) != 1 {
		t.Fatalf("own ports must win: %+v", s.Targets[0].Ports)
	}
	if len(s.Targets[1].Ports) != 2 || s.Targets[1].Ports[0] != 22 {
		t.Fatalf("global ports must backfill: %+v", s.Targets[1].Ports)
	}

	s.OnCycle([]liveness.Report{{IP: "10.0.0.5", At: 123}})
	if len(sent) != 1 || sent[0].Type != protocol.MessageTypeLivenessReport {
		t.Fatalf("sent = %+v", sent)
	}
	items, ok := sent[0].Payload["reports"].([]map[string]interface{})
	if !ok || len(items) != 1 || items[0]["ip"] != "10.0.0.5" || items[0]["at"] != int64(123) {
		t.Fatalf("reports = %v", sent[0].Payload["reports"])
	}
	arp, ok := items[0]["arp"].(map[string]interface{})
	if !ok || arp["verdict"] != "" {
		t.Fatalf("arp = %v", items[0]["arp"])
	}
	if _, has := arp["detail"]; has {
		t.Fatalf("empty detail must be omitted: %v", arp)
	}
}

// TestBuildLivenessReport 空报告清单与详情字段透传。
func TestBuildLivenessReport(t *testing.T) {
	msg := buildLivenessReport(nil)
	items := msg.Payload["reports"].([]map[string]interface{})
	if len(items) != 0 {
		t.Fatalf("empty reports = %v", items)
	}

	msg = buildLivenessReport([]liveness.Report{{
		IP:   "10.0.0.5",
		ARP:  liveness.Source{Verdict: liveness.VerdictPresent, Detail: "arp complete"},
		ICMP: liveness.Source{Verdict: liveness.VerdictUnknown},
		TCP:  liveness.Source{Verdict: liveness.VerdictPresent, Detail: "tcp 22 connected"},
		At:   1,
	}})
	items = msg.Payload["reports"].([]map[string]interface{})
	tcp := items[0]["tcp"].(map[string]interface{})
	if tcp["verdict"] != "present" || tcp["detail"] != "tcp 22 connected" {
		t.Fatalf("tcp = %v", tcp)
	}
	icmp := items[0]["icmp"].(map[string]interface{})
	if icmp["verdict"] != "unknown" {
		t.Fatalf("icmp = %v", icmp)
	}
	if _, has := icmp["detail"]; has {
		t.Fatalf("empty icmp detail must be omitted: %v", icmp)
	}
}

// TestAgentLivenessLoop 未装配探活调度时 no-op；已装配则随 agent ctx 取消退出。
func TestAgentLivenessLoop(t *testing.T) {
	(&Agent{}).livenessLoop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Agent{ctx: ctx, liveness: &liveness.Scheduler{}}).livenessLoop()
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("livenessLoop must return on ctx cancel")
	}
}

// TestStartCmdBuildConfigLiveness CLI 探活参数装配与非法参数报错。
func TestStartCmdBuildConfigLiveness(t *testing.T) {
	cfg, err := (&StartCmd{
		Server: "ws://h", LivenessTargets: "10.0.0.5,10.0.0.6",
		LivenessPorts: "22,80", LivenessInterval: 5,
	}).BuildConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LivenessTargets) != 2 || cfg.LivenessTargets[1].IP != "10.0.0.6" ||
		len(cfg.LivenessPorts) != 2 || cfg.LivenessPorts[1] != 80 || cfg.LivenessIntervalSeconds != 5 {
		t.Fatalf("cfg = %+v", cfg)
	}

	bad := map[string]*StartCmd{
		"bad target": {Server: "ws://h", LivenessTargets: "not-an-ip"},
		"bad port":   {Server: "ws://h", LivenessTargets: "10.0.0.5", LivenessPorts: "70000"},
	}
	for name, cmd := range bad {
		if _, err := cmd.BuildConfig(); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}
