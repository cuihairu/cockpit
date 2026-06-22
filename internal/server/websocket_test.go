package server

import (
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

func TestDecodeRegisterForWebSocketAllowsNilPayload(t *testing.T) {
	got, err := decodeRegisterForWebSocket(&protocol.Message{
		Type:    protocol.MessageTypeRegister,
		Payload: nil,
	})
	if err != nil {
		t.Fatalf("decodeRegisterForWebSocket() error = %v", err)
	}
	if got.AgentID != "" {
		t.Fatalf("AgentID = %q, want empty before registration assigns ID", got.AgentID)
	}
}

func TestRegisterAgentConnectionCreatesNewAgent(t *testing.T) {
	s := newTestServerWithDB(t)

	reg := &protocol.RegisterPayload{
		AgentID:  "agent-1",
		Hostname: "node-1",
		IP:       "10.0.0.10",
		Location: protocol.Location{Region: "local", Zone: "rack-a"},
		Capabilities: []protocol.Capability{
			{Type: "system", Version: "1"},
		},
	}

	agent, rejection, err := s.registerAgentConnection(nil, reg)
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("registerAgentConnection() rejection = %#v", rejection)
	}
	if agent.ID != "agent-1" {
		t.Fatalf("agent ID = %q, want agent-1", agent.ID)
	}

	stored, err := s.db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if stored.Status != "online" {
		t.Errorf("stored status = %q, want online", stored.Status)
	}
	if stored.Region != "local" || stored.Zone != "rack-a" {
		t.Errorf("stored location = %s/%s, want local/rack-a", stored.Region, stored.Zone)
	}
}

func TestRegisterAgentConnectionGeneratesAgentID(t *testing.T) {
	s := newTestServerWithDB(t)

	reg := &protocol.RegisterPayload{Hostname: "anonymous"}
	agent, rejection, err := s.registerAgentConnection(nil, reg)
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("registerAgentConnection() rejection = %#v", rejection)
	}
	if !strings.HasPrefix(agent.ID, "agent-") {
		t.Fatalf("generated ID = %q, want agent-*", agent.ID)
	}
	if reg.AgentID != agent.ID {
		t.Fatalf("registration payload AgentID = %q, want %q", reg.AgentID, agent.ID)
	}
}

func TestRegisterAgentConnectionRejectsDuplicateActiveAgent(t *testing.T) {
	s := newTestServerWithDB(t)
	if err := s.registry.Register(NewAgent("agent-1", nil)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	agent, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if agent != nil {
		t.Fatalf("agent = %#v, want nil", agent)
	}
	if rejection == nil {
		t.Fatal("rejection = nil, want duplicate_connection")
	}
	if rejection.code != "duplicate_connection" {
		t.Fatalf("rejection code = %q, want duplicate_connection", rejection.code)
	}
}

func TestAuthenticateAgentRegistrationRejectsMissingSecret(t *testing.T) {
	s := newTestServerWithDB(t)
	secret, err := storage.GenerateAgentSecret()
	if err != nil {
		t.Fatalf("GenerateAgentSecret() error = %v", err)
	}
	hash, err := storage.HashAgentSecret(secret)
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-1", SecretHash: hash}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-1"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing == nil {
		t.Fatal("existing = nil, want stored agent")
	}
	if rejection == nil {
		t.Fatal("rejection = nil, want authentication_required")
	}
	if rejection.code != "authentication_required" {
		t.Fatalf("rejection code = %q, want authentication_required", rejection.code)
	}
}

func TestAuthenticateAgentRegistrationRejectsInvalidSecret(t *testing.T) {
	s := newTestServerWithDB(t)
	secret, err := storage.GenerateAgentSecret()
	if err != nil {
		t.Fatalf("GenerateAgentSecret() error = %v", err)
	}
	hash, err := storage.HashAgentSecret(secret)
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-1", SecretHash: hash}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	_, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{
		AgentID: "agent-1",
		Secret:  "wrong-secret",
	})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if rejection == nil {
		t.Fatal("rejection = nil, want authentication_failed")
	}
	if rejection.code != "authentication_failed" {
		t.Fatalf("rejection code = %q, want authentication_failed", rejection.code)
	}
}

func TestAuthenticateAgentRegistrationAcceptsValidSecret(t *testing.T) {
	s := newTestServerWithDB(t)
	secret, err := storage.GenerateAgentSecret()
	if err != nil {
		t.Fatalf("GenerateAgentSecret() error = %v", err)
	}
	hash, err := storage.HashAgentSecret(secret)
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-1", SecretHash: hash}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{
		AgentID: "agent-1",
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("rejection = %#v, want nil", rejection)
	}
	if existing == nil || existing.SecretHash != hash {
		t.Fatal("existing agent with secret hash should be returned")
	}
}

func TestRegisterAgentConnectionPreservesExistingSecretHash(t *testing.T) {
	s := newTestServerWithDB(t)
	secret, err := storage.GenerateAgentSecret()
	if err != nil {
		t.Fatalf("GenerateAgentSecret() error = %v", err)
	}
	hash, err := storage.HashAgentSecret(secret)
	if err != nil {
		t.Fatalf("HashAgentSecret() error = %v", err)
	}
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-1", SecretHash: hash}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	_, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{
		AgentID:  "agent-1",
		Hostname: "node-1",
		Secret:   secret,
	})
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("registerAgentConnection() rejection = %#v", rejection)
	}

	stored, err := s.db.GetAgent("agent-1")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if stored.SecretHash != hash {
		t.Fatal("existing secret hash was not preserved")
	}
}
