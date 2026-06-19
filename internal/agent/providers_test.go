package agent

import (
	"os"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// withEnv sets env vars for the duration of the test and restores the previous
// values (or unsets them) on cleanup. Keeps provider tests hermetic.
func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	prev := make(map[string]string, len(kv))
	for k := range kv {
		prev[k], _ = os.LookupEnv(k)
	}
	for k, v := range kv {
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
	}
	t.Cleanup(func() {
		for k, v := range prev {
			if _, ok := os.LookupEnv(k); ok || v != "" {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	})
}

func TestSetupProviders_SystemAlwaysRegistered(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})
	a.setupProviders()

	got := a.rpc.RegisteredTypes()
	if len(got) != 1 || got[0] != "system" {
		t.Fatalf("expected only [system] registered, got %v", got)
	}
}

func TestSetupProviders_NilRpcIsNoop(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})
	a.rpc = nil
	// Should not panic.
	a.setupProviders()
}

func TestSetupProviders_PVERegisteredWithEnv(t *testing.T) {
	// PVE provider constructor is pure (no I/O), so we can assert registration.
	withEnv(t, map[string]string{
		"PVE_URL":          "https://pve.test:8006",
		"PVE_TOKEN_ID":     "root@pam!test",
		"PVE_TOKEN_SECRET": "secret-value",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "pve-api"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"pve", "system"})
}

func TestSetupProviders_PVESkippedWhenCapabilityMissing(t *testing.T) {
	withEnv(t, map[string]string{
		"PVE_URL":          "https://pve.test:8006",
		"PVE_TOKEN_ID":     "root@pam!test",
		"PVE_TOKEN_SECRET": "secret-value",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	// No pve-api capability → PVE must not register even with env vars set.
	a.capabilities = nil
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

func TestSetupProviders_PVESkippedWhenEnvMissing(t *testing.T) {
	// Strip every PVE env var so the skip branch is exercised.
	withEnv(t, map[string]string{
		"PVE_URL":          "",
		"PVE_TOKEN_ID":     "",
		"PVE_TOKEN_SECRET": "",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "pve-api"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

func TestSetupProviders_PVESkippedWhenOnlyTokenIDPresent(t *testing.T) {
	// Endpoint comes from capability, but token secret is required.
	withEnv(t, map[string]string{
		"PVE_URL":          "",
		"PVE_TOKEN_ID":     "root@pam!test",
		"PVE_TOKEN_SECRET": "",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "pve-api", Endpoint: "https://pve.test:8006"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

func TestSetupProviders_OpenWrtRegisteredWithEnv(t *testing.T) {
	// OpenWrt constructor is pure (no I/O), so we can assert registration.
	withEnv(t, map[string]string{
		"OPENWRT_HOST": "192.168.1.1",
		"OPENWRT_USER": "root",
		"OPENWRT_PASS": "password",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "openwrt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"openwrt", "system"})
}

func TestSetupProviders_OpenWrtCustomPort(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENWRT_HOST": "192.168.1.1",
		"OPENWRT_PORT": "8443",
		"OPENWRT_USER": "root",
		"OPENWRT_PASS": "password",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "openwrt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"openwrt", "system"})
}

func TestSetupProviders_OpenWrtSkippedWhenEnvMissing(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENWRT_HOST": "",
		"OPENWRT_USER": "",
		"OPENWRT_PASS": "",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "openwrt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

func TestSetupProviders_OpenWrtInvalidPortIgnored(t *testing.T) {
	// Invalid OPENWRT_PORT falls back to default 443; provider still registers.
	withEnv(t, map[string]string{
		"OPENWRT_HOST": "192.168.1.1",
		"OPENWRT_PORT": "not-a-number",
		"OPENWRT_USER": "root",
		"OPENWRT_PASS": "password",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "openwrt"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"openwrt", "system"})
}

func TestSetupProviders_DockerSkippedWhenNoEndpoint(t *testing.T) {
	// Docker constructor does a Ping, so without a real daemon the provider
	// cannot be registered. Assert the env-detection skip path instead: no
	// endpoint anywhere means registerDockerProvider bails before constructing.
	withEnv(t, map[string]string{"DOCKER_HOST": ""})

	a := NewAgent(Config{ServerURL: "ws://test"})
	// docker-api capability with empty Endpoint and no DOCKER_HOST.
	a.capabilities = []protocol.Capability{{Type: "docker-api"}}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

func TestSetupProviders_DockerUsesCapabilityEndpoint(t *testing.T) {
	// When Endpoint is on the capability, DOCKER_HOST is not consulted.
	// With a bogus unix socket the docker Ping fails, so only system should
	// remain — but importantly the code path that reads cap.Endpoint is taken.
	withEnv(t, map[string]string{"DOCKER_HOST": ""})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "docker-api", Endpoint: "unix:///definitely-not-a-real-socket"},
	}
	a.setupProviders()

	// Docker client construction fails on Ping → docker not registered.
	got := a.rpc.RegisteredTypes()
	for _, typ := range got {
		if typ == "docker" {
			t.Fatalf("docker should not have registered without a reachable daemon, got %v", got)
		}
	}
}

func TestSetupProviders_DockerUsesDockerHostEnv(t *testing.T) {
	// No Endpoint on capability, but DOCKER_HOST is set. Construction still
	// fails (no real daemon), but the env-fallback branch is exercised.
	withEnv(t, map[string]string{"DOCKER_HOST": "unix:///definitely-not-a-real-socket"})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{{Type: "docker-api"}}
	a.setupProviders()

	got := a.rpc.RegisteredTypes()
	for _, typ := range got {
		if typ == "docker" {
			t.Fatalf("docker should not have registered without a reachable daemon, got %v", got)
		}
	}
}

func TestSetupProviders_MultipleCapabilities(t *testing.T) {
	// pve + openwrt both register; docker without daemon is skipped.
	withEnv(t, map[string]string{
		"PVE_URL":          "https://pve.test:8006",
		"PVE_TOKEN_ID":     "root@pam!test",
		"PVE_TOKEN_SECRET": "secret",
		"OPENWRT_HOST":     "192.168.1.1",
		"OPENWRT_USER":     "root",
		"OPENWRT_PASS":     "pass",
		"DOCKER_HOST":      "",
	})

	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "pve-api", Endpoint: "https://pve.test:8006"},
		{Type: "openwrt"},
		{Type: "docker-api"},
	}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"openwrt", "pve", "system"})
}

func TestSetupProviders_UnknownCapabilityIgnored(t *testing.T) {
	a := NewAgent(Config{ServerURL: "ws://test"})
	a.capabilities = []protocol.Capability{
		{Type: "hardware-monitor"},
		{Type: "network-monitor"},
		{Type: "unknown-future-cap"},
	}
	a.setupProviders()

	assertRegistered(t, a.rpc.RegisteredTypes(), []string{"system"})
}

// assertRegistered checks that got contains exactly the want types (both sorted).
func assertRegistered(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("registered types mismatch:\n got: %v\nwant: %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("registered types mismatch:\n got: %v\nwant: %v", got, want)
		}
	}
}
