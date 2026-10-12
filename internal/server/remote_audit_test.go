package server

import (
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
)

func TestValidateRemoteTargetDefaultRejectsExplicitlyConfiguredTargetsOnly(t *testing.T) {
	s := &Server{}

	allow, reason := s.validateRemoteTarget("10.0.0.10")
	if allow {
		t.Fatal("validateRemoteTarget() allowed target without allow-list")
	}
	if !strings.Contains(reason, "remote_control.allowed_targets") {
		t.Fatalf("reason = %q, want allowed_targets guidance", reason)
	}
	if strings.Contains(reason, "inventory") {
		t.Fatalf("reason = %q, should not mention unsupported inventory allow-list", reason)
	}
}

func TestValidateRemoteTargetAllowsConfiguredTarget(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"10.0.0.10", "host.local"},
			},
		},
	}

	allow, reason := s.validateRemoteTarget(" 10.0.0.10 ")
	if !allow {
		t.Fatalf("validateRemoteTarget() rejected configured target: %s", reason)
	}
}

func TestValidateRemoteTargetAllowsConfiguredCIDR(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"192.168.10.0/24"},
			},
		},
	}

	allow, reason := s.validateRemoteTarget("192.168.10.42")
	if !allow {
		t.Fatalf("validateRemoteTarget() rejected configured CIDR target: %s", reason)
	}
}

func TestValidateRemoteTargetRejectsHostForCIDRRule(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"192.168.10.0/24"},
			},
		},
	}

	allow, reason := s.validateRemoteTarget("db.internal")
	if allow {
		t.Fatal("validateRemoteTarget() allowed hostname for CIDR-only allow-list")
	}
	if !strings.Contains(reason, "host/IP/CIDR") {
		t.Fatalf("reason = %q, want CIDR guidance", reason)
	}
}

func TestValidateRemoteTargetAllowsArbitraryWhenConfigured(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowArbitraryTarget: true,
			},
		},
	}

	allow, reason := s.validateRemoteTarget("unlisted.internal")
	if !allow {
		t.Fatalf("validateRemoteTarget() rejected arbitrary target: %s", reason)
	}
}

func TestValidateRemoteTargetForAgentAllowsMatchingPolicy(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"192.168.10.0/24"},
				EgressPolicies: []*config.RemoteEgressPolicy{
					{
						AgentID:        "office-agent",
						AllowedTargets: []string{"192.168.10.0/24"},
						AllowedPorts:   []int{22, 3389},
					},
				},
			},
		},
	}

	allow, reason := s.validateRemoteTargetForAgent("office-agent", "192.168.10.42", 22)
	if !allow {
		t.Fatalf("validateRemoteTargetForAgent() rejected matching policy: %s", reason)
	}
}

func TestValidateRemoteTargetForAgentRejectsMismatchedAgent(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"192.168.10.0/24"},
				EgressPolicies: []*config.RemoteEgressPolicy{
					{
						AgentID:        "office-agent",
						AllowedTargets: []string{"192.168.10.0/24"},
						AllowedPorts:   []int{22},
					},
				},
			},
		},
	}

	allow, reason := s.validateRemoteTargetForAgent("home-agent", "192.168.10.42", 22)
	if allow {
		t.Fatal("validateRemoteTargetForAgent() allowed agent without egress policy")
	}
	if !strings.Contains(reason, "remote_control.egress") {
		t.Fatalf("reason = %q, want egress guidance", reason)
	}
}

func TestValidateRemoteTargetForAgentRejectsMismatchedPort(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			RemoteControl: &config.RemoteControlConfig{
				AllowedTargets: []string{"192.168.10.0/24"},
				EgressPolicies: []*config.RemoteEgressPolicy{
					{
						AgentID:        "office-agent",
						AllowedTargets: []string{"192.168.10.0/24"},
						AllowedPorts:   []int{22},
					},
				},
			},
		},
	}

	allow, reason := s.validateRemoteTargetForAgent("office-agent", "192.168.10.42", 3389)
	if allow {
		t.Fatal("validateRemoteTargetForAgent() allowed disallowed port")
	}
	if !strings.Contains(reason, "allowed_ports") {
		t.Fatalf("reason = %q, want allowed_ports guidance", reason)
	}
}

// 多策略命中规则（D-2026-10-12-2）：同一 agent 多条策略按「任一策略同时
// 放行目标与端口即放行」合并，与配置顺序无关；不跨策略拼凑。
func TestMatchRemoteEgressMultiPolicyUnion(t *testing.T) {
	policies := []*config.RemoteEgressPolicy{
		{AgentID: "gw", AllowedTargets: []string{"10.0.0.0/24"}, AllowedPorts: []int{22}},
		{AgentID: "gw", AllowedTargets: []string{"db.internal"}, AllowedPorts: []int{5900}},
	}
	newServer := func() *Server {
		return &Server{cfg: &config.Config{RemoteControl: &config.RemoteControlConfig{
			AllowedTargets: []string{"10.0.0.0/24", "db.internal"},
			EgressPolicies: policies,
		}}}
	}

	// 各策略各自的 (目标, 端口) 组合放行
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		s := newServer()
		s.cfg.RemoteControl.EgressPolicies = []*config.RemoteEgressPolicy{policies[order[0]], policies[order[1]]}
		if m, reason := s.matchRemoteEgress("gw", "10.0.0.5", 22); m == nil {
			t.Fatalf("policy A pair rejected (order %v): %s", order, reason)
		}
		if m, reason := s.matchRemoteEgress("gw", "db.internal", 5900); m == nil {
			t.Fatalf("policy B pair rejected (order %v): %s", order, reason)
		}
		// 跨策略拼凑拒绝：A 的目标 × B 的端口
		if m, _ := s.matchRemoteEgress("gw", "10.0.0.5", 5900); m != nil {
			t.Fatalf("cross-policy pair allowed (order %v)", order)
		}
		if m, reason := s.matchRemoteEgress("gw", "db.internal", 22); m != nil {
			t.Fatalf("cross-policy pair allowed (order %v): %s", order, reason)
		}
		// 目标命中但端口两条策略都不放行 → 端口文案
		if m, reason := s.matchRemoteEgress("gw", "db.internal", 8080); m != nil || !strings.Contains(reason, "allowed_ports") {
			t.Fatalf("pairless port: m=%v reason=%q", m, reason)
		}
	}
}

func TestMatchRemoteEgressNoPolicyForAgent(t *testing.T) {
	s := &Server{cfg: &config.Config{RemoteControl: &config.RemoteControlConfig{
		AllowedTargets: []string{"10.0.0.0/24"},
		EgressPolicies: []*config.RemoteEgressPolicy{
			{AgentID: "other", AllowedTargets: []string{"10.0.0.0/24"}, AllowedPorts: []int{22}},
		},
	}}}

	allow, reason := s.validateRemoteTargetForAgent("gw", "10.0.0.5", 22)
	if allow {
		t.Fatal("agent without any policy must be denied")
	}
	if !strings.Contains(reason, "add it to remote_control.egress") || strings.Contains(reason, "allowed_targets") {
		t.Fatalf("reason = %q, want agent-level egress guidance", reason)
	}
}
