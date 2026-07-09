package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestParseDockerContainerRoutes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"list", http.MethodGet, "/api/docker/agents/a1/containers?all=true", "docker.containers.list"},
		{"get", http.MethodGet, "/api/docker/agents/a1/containers/abc", "docker.containers.get"},
		{"start", http.MethodPost, "/api/docker/agents/a1/containers/abc/start", "docker.containers.start"},
		{"stop", http.MethodPost, "/api/docker/agents/a1/containers/abc/stop?timeout=5", "docker.containers.stop"},
		{"remove", http.MethodDelete, "/api/docker/agents/a1/containers/abc?force=true&volumes=true", "docker.containers.remove"},
		{"logs", http.MethodGet, "/api/docker/agents/a1/containers/abc/logs?tail=100", "docker.containers.logs"},
		{"stats", http.MethodGet, "/api/docker/agents/a1/containers/abc/stats", "docker.containers.stats"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			agentID, method, params, err := parseDockerRequest(req)
			if err != nil {
				t.Fatalf("parseDockerRequest() error = %v", err)
			}
			if agentID != "a1" {
				t.Fatalf("agentID = %q, want a1", agentID)
			}
			if method != tt.want {
				t.Fatalf("method = %q, want %q", method, tt.want)
			}
			if tt.name != "list" && params["id"] != "abc" {
				t.Fatalf("id = %v, want abc", params["id"])
			}
		})
	}
}

func TestParseDockerResourceRoutes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"images list", http.MethodGet, "/api/docker/agents/a1/images", "docker.images.list"},
		{"images pull", http.MethodPost, "/api/docker/agents/a1/images/pull?ref=nginx:latest", "docker.images.pull"},
		{"images remove", http.MethodDelete, "/api/docker/agents/a1/images/nginx%3Alatest?force=true", "docker.images.remove"},
		{"volumes list", http.MethodGet, "/api/docker/agents/a1/volumes", "docker.volumes.list"},
		{"volumes remove", http.MethodDelete, "/api/docker/agents/a1/volumes/data", "docker.volumes.remove"},
		{"networks list", http.MethodGet, "/api/docker/agents/a1/networks", "docker.networks.list"},
		{"system info", http.MethodGet, "/api/docker/agents/a1/system/info", "docker.system.info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			_, method, _, err := parseDockerRequest(req)
			if err != nil {
				t.Fatalf("parseDockerRequest() error = %v", err)
			}
			if method != tt.want {
				t.Fatalf("method = %q, want %q", method, tt.want)
			}
		})
	}
}

func TestParseDockerRequestAllowsEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/docker/agents/a1/containers/abc/start", strings.NewReader(""))

	agentID, method, params, err := parseDockerRequest(req)
	if err != nil {
		t.Fatalf("parseDockerRequest() error = %v", err)
	}
	if agentID != "a1" {
		t.Fatalf("agentID = %q, want a1", agentID)
	}
	if method != "docker.containers.start" {
		t.Fatalf("method = %q, want docker.containers.start", method)
	}
	if params["id"] != "abc" {
		t.Fatalf("id = %v, want abc", params["id"])
	}
}

func TestHandleDockerRequiresCapability(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/docker/agents/a1/containers", nil)
	rec := httptest.NewRecorder()

	s.handleDocker(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDockerForwardsRPC(t *testing.T) {
	s := newTestServerWithDB(t)
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "docker-api"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		reqMsg := <-agent.Send
		if reqMsg.Type != protocol.MessageTypeRPCRequest {
			t.Errorf("message type = %s, want %s", reqMsg.Type, protocol.MessageTypeRPCRequest)
		}
		if reqMsg.Payload["method"] != "docker.containers.list" {
			t.Errorf("method = %v, want docker.containers.list", reqMsg.Payload["method"])
		}
		params := reqMsg.Payload["params"].(map[string]interface{})
		if params["all"] != true {
			t.Errorf("all = %v, want true", params["all"])
		}

		resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{
			"status": "success",
			"data": []map[string]interface{}{
				{"id": "c1", "name": "web"},
			},
		})
		resp.ID = reqMsg.ID
		s.handleRPCResponse(resp)
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/docker/agents/a1/containers?all=true", nil)
	rec := httptest.NewRecorder()

	s.handleDocker(rec, req)
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var data []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&data); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if len(data) != 1 || data[0]["id"] != "c1" {
		t.Fatalf("unexpected response: %v", data)
	}
}
