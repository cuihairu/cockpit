package register

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// TestDeriveID ID 派生正反用例：显式/自动/bias/machine-id 截断/回退随机。
func TestDeriveID(t *testing.T) {
	cases := []struct {
		name      string
		cfg       Config
		machineID string
		hostname  string
		want      string
		wantPre   string // 非精确匹配时按前缀断言
	}{
		{name: "explicit", cfg: Config{AgentID: "my-agent"}, want: "my-agent"},
		{name: "explicit with bias", cfg: Config{AgentID: "my-agent", Bias: 2}, want: "my-agent-2"},
		{name: "machine-id trimmed", cfg: Config{}, machineID: "abcdef1234567890", hostname: "h1",
			want: "agent-h1-abcdef12"},
		{name: "machine-id short", cfg: Config{}, machineID: "ab", hostname: "h1",
			want: "agent-h1-ab"},
		{name: "machine-id with bias", cfg: Config{Bias: 3}, machineID: "abcdef1234567890", hostname: "h1",
			want: "agent-h1-abcdef12-3"},
		{name: "no machine-id random", cfg: Config{}, machineID: "", hostname: "h1", wantPre: "agent-h1-"},
		{name: "no machine-id with bias", cfg: Config{Bias: 1}, machineID: "", hostname: "h1", wantPre: "agent-h1-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveID(tc.cfg, tc.machineID, tc.hostname)
			if tc.wantPre != "" {
				if !strings.HasPrefix(got, tc.wantPre) {
					t.Fatalf("DeriveID = %q, want prefix %q", got, tc.wantPre)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("DeriveID = %q, want %q", got, tc.want)
			}
		})
	}
	// bias>0 时随机回退也带后缀
	got := DeriveID(Config{Bias: 5}, "", "h1")
	if !strings.HasSuffix(got, "-5") {
		t.Fatalf("random fallback with bias = %q, want -5 suffix", got)
	}
}

// TestDetectLocation 位置矩阵（自 agent_test.go 迁移）：cfg 指定 / env
// 回退 / cfg 压 env / 单边 env / unknown 兜底。
func TestDetectLocation(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		env   map[string]string
		wantR string
		wantZ string
	}{
		{name: "from config", cfg: Config{Region: "us-west", Zone: "zone-a"},
			wantR: "us-west", wantZ: "zone-a"},
		{name: "from env", env: map[string]string{"COCKPIT_REGION": "eu-central", "COCKPIT_ZONE": "zone-b"},
			wantR: "eu-central", wantZ: "zone-b"},
		{name: "config overrides env", cfg: Config{Region: "us-west", Zone: "zone-a"},
			env:   map[string]string{"COCKPIT_REGION": "eu-central", "COCKPIT_ZONE": "zone-b"},
			wantR: "us-west", wantZ: "zone-a"},
		{name: "region env only", env: map[string]string{"COCKPIT_REGION": "eu-central"},
			wantR: "eu-central", wantZ: "unknown"},
		{name: "zone env only", env: map[string]string{"COCKPIT_ZONE": "zone-b"},
			wantR: "unknown", wantZ: "zone-b"},
		{name: "default unknown", wantR: "unknown", wantZ: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			loc := DetectLocation(tc.cfg)
			if loc.Region != tc.wantR || loc.Zone != tc.wantZ {
				t.Fatalf("DetectLocation(%+v, env %v) = %+v, want %s/%s",
					tc.cfg, tc.env, loc, tc.wantR, tc.wantZ)
			}
		})
	}
}

// regCapture 构造带报文捕获的 Registrar（传输全注入，无网络）。
func regCapture(resp *protocol.Message, respErr error, accepted *bool) (*Registrar, *[]*protocol.Message) {
	var sent []*protocol.Message
	r := New(
		Config{AgentID: "a1", Secret: "s", Region: "r", Zone: "z", Version: "v1",
			Labels: map[string]interface{}{"k": "v"}},
		Facts{
			MachineID:      func() string { return "abcdef1234567890" },
			PublicIP:       func() string { return "1.2.3.4" },
			LocalIPs:       func() []string { return []string{"10.0.0.1"} },
			Virtualization: func() interface{} { return "kvm" },
		},
		func(m *protocol.Message) error {
			sent = append(sent, m)
			return nil
		},
		func() (*protocol.Message, error) {
			if respErr != nil {
				return nil, respErr
			}
			return resp, nil
		},
		func() { *accepted = true },
	)
	return r, &sent
}

// TestRegisterSuccess 全字段报文 + 放行回调。
func TestRegisterSuccess(t *testing.T) {
	accepted := false
	r, sent := regCapture(&protocol.Message{Type: protocol.MessageTypeRegister}, nil, &accepted)

	started := time.Unix(1700000000, 0)
	id, loc, err := r.Register(started,
		[]protocol.Capability{{Type: "exec", Endpoint: "rpc"}},
		[]protocol.RemoteServicePayload{{Protocol: "ssh", Port: 22}})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id != "a1" || loc.Region != "r" || loc.Zone != "z" {
		t.Fatalf("id=%q loc=%+v", id, loc)
	}
	if !accepted {
		t.Fatal("onAccepted not called")
	}
	if len(*sent) != 1 {
		t.Fatalf("sent = %d messages", len(*sent))
	}
	msg := (*sent)[0]
	if msg.Type != protocol.MessageTypeRegister {
		t.Fatalf("type = %s", msg.Type)
	}
	p := msg.Payload
	for k, want := range map[string]interface{}{
		"agentId": "a1", "secret": "s", "version": "v1", "hostname": p["hostname"], "ip": "1.2.3.4",
	} {
		if p[k] != want {
			t.Fatalf("payload[%s] = %v, want %v", k, p[k], want)
		}
	}
	if p["startedAt"] != int64(1700000000) {
		t.Fatalf("payload[startedAt] = %v (%T)", p["startedAt"], p["startedAt"])
	}
	if _, ok := p["localIps"].([]string); !ok {
		t.Fatalf("payload[localIps] type = %T", p["localIps"])
	}
	if _, ok := p["virtualization"].(string); !ok {
		t.Fatalf("payload[virtualization] type = %T", p["virtualization"])
	}
	if _, ok := p["capabilities"].([]protocol.Capability); !ok {
		t.Fatalf("payload[capabilities] type = %T", p["capabilities"])
	}
	if _, ok := p["services"].([]protocol.RemoteServicePayload); !ok {
		t.Fatalf("payload[services] type = %T", p["services"])
	}
}

// TestRegisterWrongResponseType 响应类型不符报错、不放行。
func TestRegisterWrongResponseType(t *testing.T) {
	accepted := false
	r, _ := regCapture(&protocol.Message{Type: protocol.MessageTypeHeartbeat}, nil, &accepted)
	if _, _, err := r.Register(time.Now(), nil, nil); err == nil ||
		!strings.HasPrefix(err.Error(), "expected register response") {
		t.Fatalf("err = %v", err)
	}
	if accepted {
		t.Fatal("onAccepted must not fire on wrong response type")
	}
}

// TestRegisterWriteFailure 首包写失败原样上抛、不放行。
func TestRegisterWriteFailure(t *testing.T) {
	accepted := false
	var sent []*protocol.Message
	r := New(Config{AgentID: "a1"}, Facts{MachineID: func() string { return "" }},
		func(m *protocol.Message) error { return errors.New("queue full") },
		func() (*protocol.Message, error) { return nil, errors.New("should not read") },
		func() { accepted = true })
	if _, _, err := r.Register(time.Now(), nil, nil); err == nil || err.Error() != "queue full" {
		t.Fatalf("err = %v", err)
	}
	if accepted || len(sent) != 0 {
		t.Fatal("must fail before read")
	}
}

// TestRegisterReadFailure 等响应失败（含 conn 快照 nil）原样上抛、不放行。
func TestRegisterReadFailure(t *testing.T) {
	accepted := false
	r, _ := regCapture(nil, errors.New("agent not connected"), &accepted)
	if _, _, err := r.Register(time.Now(), nil, nil); err == nil ||
		err.Error() != "agent not connected" {
		t.Fatalf("err = %v", err)
	}
	if accepted {
		t.Fatal("onAccepted must not fire on read failure")
	}
}

// TestNilAcceptedCallback onAccepted 可省（装配面宽容）。
func TestNilAcceptedCallback(t *testing.T) {
	r := New(Config{AgentID: "a1"}, Facts{MachineID: func() string { return "" }},
		func(m *protocol.Message) error { return nil },
		func() (*protocol.Message, error) {
			return &protocol.Message{Type: protocol.MessageTypeRegister}, nil
		},
		nil)
	if _, _, err := r.Register(time.Now(), nil, nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
}
