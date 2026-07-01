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
