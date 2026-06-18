package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// handleMessage Agent -> Server 消息分发
func (s *Server) handleMessage(agent *Agent, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeHeartbeat:
		s.handleHeartbeat(agent, msg)
	case protocol.MessageTypeRPCResponse:
		s.handleRPCResponse(msg)
	case protocol.MessageTypeProxyData:
		s.handleProxyData(agent, msg)
	case protocol.MessageTypeProxyClose:
		s.handleProxyClose(agent, msg)
	case protocol.MessageTypeProxyError:
		s.handleProxyError(agent, msg)
	case protocol.MessageTypeDesktopData:
		s.HandleDesktopData(msg)
	case protocol.MessageTypeDesktopClose:
		s.HandleDesktopClose(msg)
	default:
		log.Printf("Unknown message type: %s from agent %s", msg.Type, agent.ID)
	}
}

// handleHeartbeat 处理心跳
func (s *Server) handleHeartbeat(agent *Agent, msg *protocol.Message) {
	agent.Heartbeat()

	// 类型化解码心跳负载；忽略错误（agent 可能发空 payload）
	if hb, err := protocol.DecodeHeartbeat(msg); err == nil && hb.SystemInfo != nil {
		s.handleSystemInfo(agent.ID, hb.SystemInfo)
	}

	// 发送 ACK
	resp := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"status":     "ack",
		"serverTime": time.Now().Unix(),
	})
	resp.ID = msg.ID // 关联请求ID

	select {
	case agent.Send <- resp:
	default:
		log.Printf("Agent %s send channel full", agent.ID)
	}
}

// handleRPCResponse 处理 RPC 响应
func (s *Server) handleRPCResponse(msg *protocol.Message) {
	if ch, exists := s.registry.GetPendingResponse(msg.ID); exists {
		select {
		case ch <- msg:
		default:
			log.Printf("Response channel full for message %s", msg.ID)
		}
		s.registry.UnregisterPendingResponse(msg.ID)
	}
}
