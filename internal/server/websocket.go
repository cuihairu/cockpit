package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
)

type registrationRejection struct {
	code    string
	message string
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
	resp := protocol.NewMessage(protocol.MessageTypeRegister, map[string]interface{}{
		"status":            "accepted",
		"serverTime":        time.Now().Unix(),
		"heartbeatInterval": int(30),
	})
	s.codec.WriteMessage(conn, resp)

	// 启动读写循环
	go s.readLoop(agent)
	go s.writeLoop(agent)
}

func decodeRegisterForWebSocket(msg *protocol.Message) (protocol.RegisterPayload, error) {
	if msg.Payload == nil {
		return protocol.RegisterPayload{}, nil
	}
	return protocol.DecodeRegister(msg)
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
	if _, exists := s.registry.Get(reg.AgentID); exists {
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

	if err := s.persistRegisteredAgent(agent, existingAgent); err != nil {
		log.Printf("Failed to persist agent to database: %v", err)
	}

	return agent, nil, nil
}

func (s *Server) authenticateAgentRegistration(reg *protocol.RegisterPayload) (*storage.Agent, *registrationRejection, error) {
	existingAgent, err := s.db.GetAgent(reg.AgentID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("load existing agent %s: %w", reg.AgentID, err)
	}

	if existingAgent.SecretHash == "" {
		// 兼容历史数据：已有 Agent 但未配置密钥时仍允许注册。
		log.Printf("Agent %s exists but has no secret configured", reg.AgentID)
		return existingAgent, nil, nil
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

func (s *Server) persistRegisteredAgent(agent *Agent, existingAgent *storage.Agent) error {
	storageAgent := toStorageAgent(agent)
	if existingAgent != nil && existingAgent.SecretHash != "" {
		storageAgent.SecretHash = existingAgent.SecretHash
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
	defer s.registry.Unregister(agent.ID)

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
