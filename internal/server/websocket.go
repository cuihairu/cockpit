package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/gorilla/websocket"
)

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
	var reg protocol.RegisterPayload
	payloadBytes, _ := json.Marshal(msg.Payload)
	if err := json.Unmarshal(payloadBytes, &reg); err != nil {
		log.Printf("Parse register payload failed: %v", err)
		conn.Close()
		return
	}

	agentID := reg.AgentID
	if agentID == "" {
		agentID = protocol.GenerateIDWithPrefix("agent")
	}

	// 认证检查
	existingAgent, err := s.db.GetAgent(agentID)
	if err == nil && existingAgent.SecretHash != "" {
		// Agent 已存在且有密钥配置，验证密钥
		if reg.Secret == "" {
			log.Printf("Agent %s registration failed: missing secret", agentID)
			s.sendRegisterError(conn, "authentication_required", "Agent secret is required")
			conn.Close()
			return
		}
		if !storage.VerifyAgentSecret(existingAgent.SecretHash, reg.Secret) {
			log.Printf("Agent %s registration failed: invalid secret", agentID)
			s.sendRegisterError(conn, "authentication_failed", "Invalid agent secret")
			conn.Close()
			return
		}
	} else if existingAgent != nil && existingAgent.SecretHash == "" {
		// Agent 存在但未配置密钥（旧版本迁移），需要配置
		log.Printf("Agent %s exists but has no secret configured", agentID)
	}

	// 检查 Registry 中是否已有活跃连接（Registry 中的 Agent 均为活跃状态）
	if _, exists := s.registry.Get(agentID); exists {
		log.Printf("Agent %s already has active connection, rejecting duplicate", agentID)
		s.sendRegisterError(conn, "duplicate_connection", "Agent already connected")
		conn.Close()
		return
	}
	// 创建 Agent
	agent := NewAgent(agentID, conn)
	agent.Update(&reg)

	// 注册到 Registry
	if err := s.registry.Register(agent); err != nil {
		log.Printf("Register agent failed: %v", err)
		s.sendRegisterError(conn, "registration_failed", err.Error())
		conn.Close()
		return
	}

	// 持久化到数据库
	storageAgent := toStorageAgent(agent)
	if existingAgent != nil && existingAgent.SecretHash != "" {
		// 保留现有的 SecretHash
		storageAgent.SecretHash = existingAgent.SecretHash
	}
	if err := s.db.UpsertAgent(storageAgent); err != nil {
		log.Printf("Failed to persist agent to database: %v", err)
	}

	log.Printf("Agent registered: %s at %s/%s", agentID, reg.Location.Region, reg.Location.Zone)

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
