package server

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
)

type registrationRejection struct {
	code    string
	message string
}

// hashAgentSecretForRegistration 注册时落库密钥的哈希注入点（测试可换实现
// 覆盖哈希失败分支；bcrypt 对合法输入不失败，错误分支仅供测试覆盖）。
var hashAgentSecretForRegistration = storage.HashAgentSecret

// agentRegistrationPolicy nil 安全的准入策略读取：测试直构 Server 无 cfg、
// 历史配置文件无该段时按零值处理（默认拒绝，见 2026-10-18 审核表一#1）。
func (s *Server) agentRegistrationPolicy() config.AgentRegistrationConfig {
	if s.cfg == nil || s.cfg.Server == nil || s.cfg.Server.AgentRegistration == nil {
		return config.AgentRegistrationConfig{}
	}
	return *s.cfg.Server.AgentRegistration
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isOriginAllowed 检查 WebSocket 升级请求的 Origin 是否在白名单内
func isOriginAllowed(r *http.Request) bool {
	allowed := os.Getenv("ALLOWED_ORIGINS")
	if allowed == "" {
		// 未配置白名单时允许所有来源（开发模式）
		log.Println("WARNING: ALLOWED_ORIGINS not set, accepting all WebSocket origins. Configure this in production!")
		return true
	}
	origin := r.Header.Get("Origin")
	// 空 Origin = 非浏览器客户端（cockpit-agent 等不出于浏览器，不发 Origin 头）。
	// 白名单是防浏览器侧跨站 WS 的；把这类客户端一并拒绝会让生产
	// （ALLOWED_ORIGINS 必配）下 agent 握手 403/bad handshake。浏览器永远带 Origin，
	// 仍走下方白名单。
	if origin == "" {
		return true
	}
	for _, a := range strings.Split(allowed, ",") {
		a = strings.TrimSpace(a)
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// 等待注册消息
	msg, err := s.codec.ReadMessage(conn)
	if err != nil {
		log.Printf("Read register message failed: %v", err)
		conn.Close()
		return
	}

	if msg.Type != protocol.MessageTypeRegister {
		log.Printf("First message must be register, got: %s", msg.Type)
		conn.Close()
		return
	}

	// 解析注册信息
	reg, err := decodeRegisterForWebSocket(msg)
	if err != nil {
		log.Printf("Parse register payload failed: %v", err)
		conn.Close()
		return
	}

	agent, rejection, err := s.registerAgentConnection(conn, &reg)
	if err != nil {
		log.Printf("Agent %s registration failed: %v", reg.AgentID, err)
		s.sendRegisterError(conn, "registration_failed", "Registration failed")
		conn.Close()
		return
	}
	if rejection != nil {
		log.Printf("Agent %s registration rejected: %s", reg.AgentID, rejection.message)
		s.sendRegisterError(conn, rejection.code, rejection.message)
		conn.Close()
		return
	}

	log.Printf("Agent registered: %s at %s/%s", agent.ID, reg.Location.Region, reg.Location.Zone)

	// 发送注册响应
	resp := protocol.NewMessage(protocol.MessageTypeRegister, s.registerRespPayload(agent))
	s.codec.WriteMessage(conn, resp)

	// 启动读写循环
	go s.readLoop(agent)
	go s.writeLoop(agent)

	// 服务健康探针配置补推（service-health D3）：注册上线即对齐配置——
	// 保存时 agent 离线的配置在这里落到 agent 侧
	s.goBg(func() { s.pushServiceHealthConfig(agent.ID) })
}

func decodeRegisterForWebSocket(msg *protocol.Message) (protocol.RegisterPayload, error) {
	if msg.Payload == nil {
		return protocol.RegisterPayload{}, nil
	}
	return protocol.DecodeRegister(msg)
}

// wsRegistryLookup registry 活跃连接查询。包级变量仅为测试可注入：
// 后注册者跳过 Get 检查即可确定性复现「检查时不存在、注册时已被
// 抢先」的 TOCTOU 后半程——真实并发下该窗口极窄，高负载（CI race
// 检测）后到者几乎总被 Get 先拦成 duplicate_connection，赌调度
// 会 flaky。
var wsRegistryLookup = func(r *Registry, agentID string) (*Agent, bool) {
	return r.Get(agentID)
}

func (s *Server) registerAgentConnection(conn *websocket.Conn, reg *protocol.RegisterPayload) (*Agent, *registrationRejection, error) {
	if reg.AgentID == "" {
		reg.AgentID = protocol.GenerateIDWithPrefix("agent")
	}

	existingAgent, rejection, err := s.authenticateAgentRegistration(reg)
	if err != nil {
		return nil, nil, err
	}
	if rejection != nil {
		return nil, rejection, nil
	}

	// Registry 中的 Agent 均为活跃连接，已有记录时拒绝重复连接。
	if _, exists := wsRegistryLookup(s.registry, reg.AgentID); exists {
		return nil, &registrationRejection{
			code:    "duplicate_connection",
			message: "Agent already connected",
		}, nil
	}

	agent := NewAgent(reg.AgentID, conn)
	agent.Update(reg)

	if err := s.registry.Register(agent); err != nil {
		return nil, &registrationRejection{
			code:    "registration_failed",
			message: err.Error(),
		}, nil
	}

	// 新注册且携带 secret 的 agent：落库其哈希（enrollment 或 dev 模式
	// 自带密钥均适用；存量记录由 persistRegisteredAgent 保留原哈希）。
	enrollSecret := ""
	if existingAgent == nil && reg.Secret != "" {
		enrollSecret = reg.Secret
	}
	if err := s.persistRegisteredAgent(agent, existingAgent, enrollSecret); err != nil {
		log.Printf("Failed to persist agent to database: %v", err)
		s.registry.Unregister(agent.ID)
		return nil, &registrationRejection{
			code:    "registration_failed",
			message: "Agent registration could not be persisted",
		}, nil
	}

	return agent, nil, nil
}

func (s *Server) authenticateAgentRegistration(reg *protocol.RegisterPayload) (*storage.Agent, *registrationRejection, error) {
	policy := s.agentRegistrationPolicy()

	existingAgent, err := s.db.GetAgent(reg.AgentID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// 2026-10-18 审核表一#1：未知 agent 默认拒绝。放行仅两条
			// 显式路径——allow_unknown（开发环境）或携带 enrollment_secret
			// 的首次注册（常量时间比较，防时序侧信道）。
			if policy.AllowUnknown {
				return nil, nil, nil
			}
			if policy.EnrollmentSecret != "" &&
				subtle.ConstantTimeCompare([]byte(reg.Secret), []byte(policy.EnrollmentSecret)) == 1 {
				return nil, nil, nil
			}
			return nil, &registrationRejection{
				code: "registration_forbidden",
				message: "Unknown agent: configure server.agent_registration.enrollment_secret " +
					"or allow_unknown (dev only) to permit registration",
			}, nil
		}
		return nil, nil, fmt.Errorf("load existing agent %s: %w", reg.AgentID, err)
	}

	if existingAgent.SecretHash == "" {
		// 历史存量 agent 未配置密钥：默认拒绝（可被冒充的入口，表一#1），
		// 兼容旧部署走显式 allow_secretless。
		if policy.AllowSecretless {
			log.Printf("Agent %s has no secret configured; reconnected under allow_secretless", reg.AgentID)
			return existingAgent, nil, nil
		}
		return existingAgent, &registrationRejection{
			code: "authentication_required",
			message: "Agent has no secret configured; provision one via the agent secret API " +
				"or enable allow_secretless explicitly",
		}, nil
	}

	if reg.Secret == "" {
		return existingAgent, &registrationRejection{
			code:    "authentication_required",
			message: "Agent secret is required",
		}, nil
	}
	if !storage.VerifyAgentSecret(existingAgent.SecretHash, reg.Secret) {
		return existingAgent, &registrationRejection{
			code:    "authentication_failed",
			message: "Invalid agent secret",
		}, nil
	}

	return existingAgent, nil, nil
}

func (s *Server) persistRegisteredAgent(agent *Agent, existingAgent *storage.Agent, enrollSecret string) error {
	storageAgent := toStorageAgent(agent)
	switch {
	case existingAgent != nil && existingAgent.SecretHash != "":
		storageAgent.SecretHash = existingAgent.SecretHash
	case enrollSecret != "":
		// 新 agent 经 enrollment_secret 准入：把其哈希落库，重连走
		// 既有 per-agent 校验（enrollment 口令轮换不影响已注册 agent）。
		hash, err := hashAgentSecretForRegistration(enrollSecret)
		if err != nil {
			return fmt.Errorf("hash enrollment secret for agent %s: %w", agent.ID, err)
		}
		storageAgent.SecretHash = hash
	}
	return s.db.UpsertAgent(storageAgent)
}

// sendRegisterError 发送注册错误响应
func (s *Server) sendRegisterError(conn *websocket.Conn, code, message string) {
	resp := protocol.NewMessage(protocol.MessageTypeError, map[string]interface{}{
		"code":    code,
		"message": message,
	})
	s.codec.WriteMessage(conn, resp)
}

// readLoop 读取循环
func (s *Server) readLoop(agent *Agent) {
	defer func() {
		s.registry.Unregister(agent.ID)
		// 更新数据库状态为离线
		if err := s.db.UpdateAgentStatus(agent.ID, "offline", time.Now()); err != nil {
			log.Printf("Agent %s failed to update offline status: %v", agent.ID, err)
		}
	}()

	for {
		msg, err := s.codec.ReadMessage(agent.Conn)
		if err != nil {
			log.Printf("Agent %s read error: %v", agent.ID, err)
			return
		}

		s.handleMessage(agent, msg)
	}
}

// writeLoop 写入循环
func (s *Server) writeLoop(agent *Agent) {
	defer agent.Conn.Close()

	for msg := range agent.Send {
		if err := s.codec.WriteMessage(agent.Conn, msg); err != nil {
			log.Printf("Agent %s write error: %v", agent.ID, err)
			return
		}
	}
}
