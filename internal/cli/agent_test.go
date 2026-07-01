package cli

import (
	"reflect"
	"testing"
)

func TestAgentStartCmdBuildConfig(t *testing.T) {
	cmd := AgentStartCmd{
		Server: "ws://localhost:9000/ws",
		ID:     "agent-1",
		Secret: "secret",
		Region: "home",
		Zone:   "dc-a",
		Labels: "env=prod,services=[docker,k8s],gpu=true,count=3",
	}

	cfg, err := cmd.BuildConfig()
	if err != nil {
		t.Fatalf("BuildConfig() error = %v", err)
	}

	if cfg.ServerURL != "ws://localhost:9000/ws" {
		t.Fatalf("BuildConfig() server = %q", cfg.ServerURL)
	}

	expected := map[string]interface{}{
		"env":      "prod",
		"services": []string{"docker", "k8s"},
		"gpu":      true,
		"count":    3,
	}
	if !reflect.DeepEqual(cfg.Labels, expected) {
		t.Fatalf("BuildConfig() labels = %#v, want %#v", cfg.Labels, expected)
	}
}

func TestAgentStartCmdBuildConfigRequiresServer(t *testing.T) {
	cmd := AgentStartCmd{}

	if _, err := cmd.BuildConfig(); err == nil {
		t.Fatal("BuildConfig() expected error for missing server")
	}
}

func TestSplitLabelParts(t *testing.T) {
	input := "env=prod,services=[docker,k8s],empty=[],nested=[one, two],gpu=true"
	got := splitLabelParts(input)
	want := []string{
		"env=prod",
		"services=[docker,k8s]",
		"empty=[]",
		"nested=[one, two]",
		"gpu=true",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitLabelParts() = %#v, want %#v", got, want)
	}
}

func TestParseLabelsSkipsInvalidEntries(t *testing.T) {
	got := parseLabels("env=prod,invalid,=missing-key")

	want := map[string]interface{}{
		"env": "prod",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLabels() = %#v, want %#v", got, want)
	}
}
