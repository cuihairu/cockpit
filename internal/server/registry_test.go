package server

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// ============ Registry Tests ============

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.agents == nil {
		t.Error("agents map should be initialized")
	}
	if r.pendingResponse == nil {
		t.Error("pendingResponse map should be initialized")
	}
	if r.timeouts.Heartbeat != 60*time.Second {
		t.Errorf("Heartbeat timeout = %v, want 60s", r.timeouts.Heartbeat)
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	agent := &Agent{ID: "agent-1", Send: make(chan *protocol.Message, 256)}

	if err := r.Register(agent); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, exists := r.agents["agent-1"]; !exists {
		t.Error("agent should be registered")
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	agent := &Agent{ID: "agent-1", Send: make(chan *protocol.Message, 256)}

	r.Register(agent)
	err := r.Register(agent)
	if err != ErrAgentAlreadyExists {
		t.Errorf("expected ErrAgentAlreadyExists, got %v", err)
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()
	agent := NewAgent("agent-1", nil)
	r.Register(agent)

	r.Unregister("agent-1")

	if _, exists := r.agents["agent-1"]; exists {
		t.Error("agent should be unregistered")
	}
}

func TestRegistryUnregisterNotFound(t *testing.T) {
	r := NewRegistry()
	r.Unregister("non-existent") // should not panic
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	agent := &Agent{ID: "agent-1", Send: make(chan *protocol.Message, 256)}
	r.Register(agent)

	found, exists := r.Get("agent-1")
	if !exists {
		t.Error("agent should exist")
	}
	if found.ID != "agent-1" {
		t.Errorf("ID = %v", found.ID)
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, exists := r.Get("non-existent")
	if exists {
		t.Error("should not exist")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Agent{ID: "a1", Send: make(chan *protocol.Message, 256)})
	r.Register(&Agent{ID: "a2", Send: make(chan *protocol.Message, 256)})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("List() count = %d, want 2", len(list))
	}
}

func TestRegistryListEmpty(t *testing.T) {
	r := NewRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("List() count = %d, want 0", len(list))
	}
}

func TestRegistryListByLocation(t *testing.T) {
	r := NewRegistry()

	agent1 := &Agent{
		ID:   "a1",
		Send: make(chan *protocol.Message, 256),
		Location: protocol.Location{Region: "us-east", Zone: "us-east-1a"},
	}
	agent2 := &Agent{
		ID:   "a2",
		Send: make(chan *protocol.Message, 256),
		Location: protocol.Location{Region: "eu-west", Zone: "eu-west-1a"},
	}
	r.Register(agent1)
	r.Register(agent2)

	// Filter by region
	found := r.ListByLocation("us-east", "")
	if len(found) != 1 {
		t.Errorf("ListByLocation(us-east) count = %d, want 1", len(found))
	}

	// Filter by region and zone
	found = r.ListByLocation("us-east", "us-east-1a")
	if len(found) != 1 {
		t.Errorf("ListByLocation(us-east, us-east-1a) count = %d, want 1", len(found))
	}

	// No match
	found = r.ListByLocation("ap-south", "")
	if len(found) != 0 {
		t.Errorf("ListByLocation(ap-south) count = %d, want 0", len(found))
	}

	// Empty filters = all
	found = r.ListByLocation("", "")
	if len(found) != 2 {
		t.Errorf("ListByLocation(empty) count = %d, want 2", len(found))
	}
}

func TestRegistryListByCapability(t *testing.T) {
	r := NewRegistry()

	agent1 := &Agent{
		ID:           "a1",
		Send:         make(chan *protocol.Message, 256),
		Capabilities: []protocol.Capability{{Type: "proxy"}, {Type: "docker"}},
	}
	agent2 := &Agent{
		ID:           "a2",
		Send:         make(chan *protocol.Message, 256),
		Capabilities: []protocol.Capability{{Type: "pve"}},
	}
	r.Register(agent1)
	r.Register(agent2)

	found := r.ListByCapability("docker")
	if len(found) != 1 {
		t.Errorf("ListByCapability(docker) count = %d, want 1", len(found))
	}

	found = r.ListByCapability("proxy")
	if len(found) != 1 {
		t.Errorf("ListByCapability(proxy) count = %d, want 1", len(found))
	}

	found = r.ListByCapability("nonexistent")
	if len(found) != 0 {
		t.Errorf("ListByCapability(nonexistent) count = %d, want 0", len(found))
	}
}

func TestRegistryUpdateHeartbeat(t *testing.T) {
	r := NewRegistry()
	agent := &Agent{
		ID:       "a1",
		Send:     make(chan *protocol.Message, 256),
		LastSeen: time.Now().Add(-5 * time.Minute),
	}
	r.Register(agent)

	result := r.UpdateHeartbeat("a1")
	if !result {
		t.Error("UpdateHeartbeat should return true")
	}

	// Verify LastSeen was updated
	found, _ := r.Get("a1")
	if time.Since(found.LastSeen) > time.Second {
		t.Error("LastSeen should be updated")
	}
}

func TestRegistryUpdateHeartbeatNotFound(t *testing.T) {
	r := NewRegistry()
	result := r.UpdateHeartbeat("nonexistent")
	if result {
		t.Error("UpdateHeartbeat should return false for unknown agent")
	}
}

func TestRegistryCleanupOffline(t *testing.T) {
	r := NewRegistry()

	online := &Agent{
		ID:       "online",
		Send:     make(chan *protocol.Message, 256),
		LastSeen: time.Now(),
	}
	offline := &Agent{
		ID:       "offline",
		Send:     make(chan *protocol.Message, 256),
		LastSeen: time.Now().Add(-5 * time.Minute),
	}
	r.Register(online)
	r.Register(offline)

	removed := r.CleanupOffline()
	if len(removed) != 1 || removed[0] != "offline" {
		t.Errorf("removed = %v, want [offline]", removed)
	}

	if _, exists := r.Get("online"); !exists {
		t.Error("online agent should still be registered")
	}
	if _, exists := r.Get("offline"); exists {
		t.Error("offline agent should be removed")
	}
}

func TestRegistryPendingResponse(t *testing.T) {
	r := NewRegistry()

	ch := make(chan *protocol.Message, 1)
	r.RegisterPendingResponse("msg-1", ch)

	found, exists := r.GetPendingResponse("msg-1")
	if !exists {
		t.Error("pending response should exist")
	}
	if found != ch {
		t.Error("should return same channel")
	}

	r.UnregisterPendingResponse("msg-1")
	_, exists = r.GetPendingResponse("msg-1")
	if exists {
		t.Error("pending response should be removed")
	}
}

func TestRegistryGetPendingResponseNotFound(t *testing.T) {
	r := NewRegistry()
	_, exists := r.GetPendingResponse("nonexistent")
	if exists {
		t.Error("should not exist")
	}
}

// ============ Agent Tests ============

func TestNewAgent(t *testing.T) {
	agent := NewAgent("agent-1", nil)
	if agent.ID != "agent-1" {
		t.Errorf("ID = %v", agent.ID)
	}
	if agent.Send == nil {
		t.Error("Send channel should be initialized")
	}
	if agent.LastSeen.IsZero() {
		t.Error("LastSeen should be set")
	}
}

func TestAgentUpdate(t *testing.T) {
	agent := NewAgent("agent-1", nil)
	agent.Update(&protocol.RegisterPayload{
		Hostname: "web-server",
		IP:       "10.0.0.1",
		Location: protocol.Location{Region: "us-east", Zone: "us-east-1a"},
		Capabilities: []protocol.Capability{
			{Type: "proxy"},
			{Type: "docker"},
		},
	})

	if agent.Hostname != "web-server" {
		t.Errorf("Hostname = %v", agent.Hostname)
	}
	if agent.IP != "10.0.0.1" {
		t.Errorf("IP = %v", agent.IP)
	}
	loc := agent.GetLocation()
	if loc.Region != "us-east" {
		t.Errorf("Region = %v", loc.Region)
	}
}

func TestAgentGetLocation(t *testing.T) {
	agent := NewAgent("a1", nil)
	agent.Location = protocol.Location{Region: "eu-west", Zone: "eu-west-1a"}

	loc := agent.GetLocation()
	if loc.Region != "eu-west" {
		t.Errorf("Region = %v", loc.Region)
	}
}

func TestAgentGetCapabilities(t *testing.T) {
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "proxy"}, {Type: "docker"}}

	caps := agent.GetCapabilities()
	if len(caps) != 2 {
		t.Errorf("count = %d, want 2", len(caps))
	}
}

func TestAgentHasCapability(t *testing.T) {
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{{Type: "proxy"}, {Type: "docker"}}

	if !agent.HasCapability("proxy") {
		t.Error("should have proxy capability")
	}
	if agent.HasCapability("pve") {
		t.Error("should not have pve capability")
	}
}

func TestAgentGetCapability(t *testing.T) {
	agent := NewAgent("a1", nil)
	agent.Capabilities = []protocol.Capability{
		{Type: "proxy", Endpoint: "0.0.0.0:8080"},
		{Type: "docker"},
	}

	cap := agent.GetCapability("proxy")
	if cap == nil {
		t.Fatal("should find proxy capability")
	}
	if cap.Endpoint != "0.0.0.0:8080" {
		t.Errorf("Endpoint = %v", cap.Endpoint)
	}

	cap = agent.GetCapability("nonexistent")
	if cap != nil {
		t.Error("should return nil for nonexistent capability")
	}
}

func TestAgentHeartbeat(t *testing.T) {
	agent := NewAgent("a1", nil)
	oldLastSeen := agent.LastSeen

	time.Sleep(time.Millisecond)
	agent.Heartbeat()

	if !agent.LastSeen.After(oldLastSeen) {
		t.Error("Heartbeat should update LastSeen")
	}
}

func TestAgentIsOnline(t *testing.T) {
	agent := NewAgent("a1", nil)

	if !agent.IsOnline(60 * time.Second) {
		t.Error("freshly created agent should be online")
	}

	agent.LastSeen = time.Now().Add(-5 * time.Minute)
	if agent.IsOnline(60 * time.Second) {
		t.Error("agent with old heartbeat should be offline")
	}
}

func TestAgentAgentID(t *testing.T) {
	agent := NewAgent("agent-123", nil)
	if agent.AgentID() != "agent-123" {
		t.Errorf("AgentID() = %v", agent.AgentID())
	}
}

func TestAgentSendMessage(t *testing.T) {
	agent := NewAgent("a1", nil)
	msg := protocol.NewMessage("test", nil)

	err := agent.SendMessage(msg)
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	// Verify message was sent to channel
	select {
	case received := <-agent.Send:
		if received.Type != "test" {
			t.Errorf("received type = %v", received.Type)
		}
	default:
		t.Error("message should be in Send channel")
	}
}

func TestAgentSendMessageChannelFull(t *testing.T) {
	agent := NewAgent("a1", nil)
	// Fill the channel (buffer is 256)
	for i := 0; i < 256; i++ {
		agent.Send <- protocol.NewMessage("fill", nil)
	}

	err := agent.SendMessage(protocol.NewMessage("overflow", nil))
	if err == nil {
		t.Error("expected error when channel is full")
	}
}

// ============ Error Tests ============

func TestErrorDefinitions(t *testing.T) {
	if ErrAgentAlreadyExists.Code != "agent_exists" {
		t.Errorf("ErrAgentAlreadyExists.Code = %v", ErrAgentAlreadyExists.Code)
	}
	if ErrAgentNotFound.Code != "agent_not_found" {
		t.Errorf("ErrAgentNotFound.Code = %v", ErrAgentNotFound.Code)
	}
}

func TestErrorError(t *testing.T) {
	e := &Error{Code: "test_code", Message: "test message"}
	expected := "test_code: test message"
	if e.Error() != expected {
		t.Errorf("Error() = %q, want %q", e.Error(), expected)
	}
}
