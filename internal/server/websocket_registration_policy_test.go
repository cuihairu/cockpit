package server

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 准入策略回归（2026-10-18 审核表一#1）：未知 agent 与无凭据存量 agent
// 默认拒绝；enrollment_secret 首注册落库哈希，重连走既有 per-agent 校验。

var errTestHashUnavailable = errors.New("hash unavailable (test)")

func TestAuthenticateAgentRegistrationRejectsUnknownAgentByDefault(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{})

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing != nil {
		t.Fatalf("existing = %#v, want nil", existing)
	}
	if rejection == nil {
		t.Fatal("rejection = nil, want registration_forbidden")
	}
	if rejection.code != "registration_forbidden" {
		t.Fatalf("rejection code = %q, want registration_forbidden", rejection.code)
	}
}

func TestAuthenticateAgentRegistrationAllowsUnknownInDevMode(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{AllowUnknown: true})

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing != nil {
		t.Fatalf("existing = %#v, want nil", existing)
	}
	if rejection != nil {
		t.Fatalf("rejection = %#v, want nil", rejection)
	}
}

func TestAuthenticateAgentRegistrationEnrollsUnknownAgentWithSecret(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{EnrollmentSecret: "enroll-token"})

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x", Secret: "enroll-token"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing != nil {
		t.Fatalf("existing = %#v, want nil", existing)
	}
	if rejection != nil {
		t.Fatalf("rejection = %#v, want nil", rejection)
	}
}

func TestAuthenticateAgentRegistrationRejectsWrongEnrollmentSecret(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{EnrollmentSecret: "enroll-token"})

	_, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x", Secret: "wrong-token"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if rejection == nil || rejection.code != "registration_forbidden" {
		t.Fatalf("rejection = %#v, want registration_forbidden", rejection)
	}
}

func TestAuthenticateAgentRegistrationRejectsSecretlessExistingByDefault(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{})
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-legacy"}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-legacy"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing == nil {
		t.Fatal("existing = nil, want stored agent")
	}
	if rejection == nil || rejection.code != "authentication_required" {
		t.Fatalf("rejection = %#v, want authentication_required", rejection)
	}
}

func TestAuthenticateAgentRegistrationAllowsSecretlessWithFlag(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{AllowSecretless: true})
	if err := s.db.UpsertAgent(&storage.Agent{ID: "agent-legacy"}); err != nil {
		t.Fatalf("UpsertAgent() error = %v", err)
	}

	_, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-legacy"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("rejection = %#v, want nil", rejection)
	}
}

func TestRegisterAgentConnectionPersistsEnrollmentSecretHash(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{EnrollmentSecret: "enroll-token"})

	reg := &protocol.RegisterPayload{AgentID: "agent-x", Secret: "enroll-token", Hostname: "node-x"}
	agent, rejection, err := s.registerAgentConnection(nil, reg)
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if rejection != nil {
		t.Fatalf("rejection = %#v, want nil", rejection)
	}
	if agent.ID != "agent-x" {
		t.Fatalf("agent ID = %q, want agent-x", agent.ID)
	}

	stored, err := s.db.GetAgent("agent-x")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if stored.SecretHash == "" {
		t.Fatal("stored SecretHash is empty, want enrollment secret hash persisted")
	}
	if !storage.VerifyAgentSecret(stored.SecretHash, "enroll-token") {
		t.Error("VerifyAgentSecret(enrolled hash) = false, want true")
	}

	// 重连：存量 SecretHash 已落库，走 per-agent 校验分支而非 enrollment。
	if _, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x", Secret: "enroll-token"}); err != nil || rejection != nil {
		t.Fatalf("reconnect authenticate = err %v rejection %#v, want nil/nil", err, rejection)
	}
	if _, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-x", Secret: "wrong"}); err != nil || rejection == nil || rejection.code != "authentication_failed" {
		t.Fatalf("reconnect wrong secret = err %v rejection %#v, want authentication_failed", err, rejection)
	}
}

func TestRegisterAgentConnectionRejectsEnrollmentHashFailure(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{EnrollmentSecret: "enroll-token"})

	orig := hashAgentSecretForRegistration
	hashAgentSecretForRegistration = func(string) (string, error) { return "", errTestHashUnavailable }
	defer func() { hashAgentSecretForRegistration = orig }()

	agent, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{AgentID: "agent-x", Secret: "enroll-token"})
	if err != nil {
		t.Fatalf("registerAgentConnection() error = %v", err)
	}
	if agent != nil {
		t.Fatalf("agent = %#v, want nil after hash failure", agent)
	}
	if rejection == nil || rejection.code != "registration_failed" {
		t.Fatalf("rejection = %#v, want registration_failed", rejection)
	}
	if _, exists := s.registry.Get("agent-x"); exists {
		t.Fatal("agent remained in registry after hash failure")
	}
	if _, err := s.db.GetAgent("agent-x"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetAgent() error = %v, want not found", err)
	}
}

// TestRegisterAgentConnectionPolicyGuardRace 策略注入点并发安全：
// 并发注册互不影响注入的哈希函数（race 腿下暴露数据竞争）。
func TestRegisterAgentConnectionPolicyGuardRace(t *testing.T) {
	s := newTestServerWithRegistrationPolicy(t, &config.AgentRegistrationConfig{EnrollmentSecret: "enroll-token"})

	var wg sync.WaitGroup
	var mu sync.Mutex
	rejections := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, rejection, err := s.registerAgentConnection(nil, &protocol.RegisterPayload{
				AgentID: "agent-race", Secret: "wrong",
			})
			if err != nil {
				t.Errorf("registerAgentConnection() error = %v", err)
			}
			if rejection != nil && strings.Contains(rejection.code, "forbidden") {
				mu.Lock()
				rejections++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if rejections == 0 {
		t.Fatal("no registration_forbidden observed under concurrency")
	}
}

// 历史配置文件无 agent_registration 段（或测试直构 Server 无 cfg）时，
// 准入策略按零值处理：仍然 fail-closed，不允许静默放行。
func TestAuthenticateAgentRegistrationNilConfigFailsClosed(t *testing.T) {
	s := &Server{db: testServerDB(t)}

	existing, rejection, err := s.authenticateAgentRegistration(&protocol.RegisterPayload{AgentID: "agent-nil-policy"})
	if err != nil {
		t.Fatalf("authenticateAgentRegistration() error = %v", err)
	}
	if existing != nil {
		t.Fatalf("existing = %#v, want nil", existing)
	}
	if rejection == nil || rejection.code != "registration_forbidden" {
		t.Fatalf("rejection = %+v, want registration_forbidden", rejection)
	}
}
