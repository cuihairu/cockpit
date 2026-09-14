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
