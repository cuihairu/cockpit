package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
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
	case protocol.MessageTypeProbeReport:
		s.handleProbeReport(agent, msg)
	default:
		log.Printf("Unknown message type: %s from agent %s", msg.Type, agent.ID)
	}
}

// handleHeartbeat 处理心跳
func (s *Server) handleHeartbeat(agent *Agent, msg *protocol.Message) {
	agent.Heartbeat()

	// DB last_seen 回写：过期判定依据（D-2026-10-08-3 根因①——此前只在
	// 注册/断连落库，服务端重启后假在线行无从判定）。nil-db 守卫同下。
	if s.db != nil {
		if err := s.db.TouchAgentLastSeen(agent.ID, time.Now()); err != nil {
			log.Printf("Agent %s last_seen update failed: %v", agent.ID, err)
		}
	}

	// 类型化解码心跳负载；忽略错误（agent 可能发空 payload）
	if hb, err := protocol.DecodeHeartbeat(msg); err == nil {
		if hb.SystemInfo != nil {
			s.handleSystemInfo(agent.ID, hb.SystemInfo)
		}
		// 启动时刻 + 服务面：内存侧刷新 + 落库（心跳是唯一 30s 级刷新通道）
		if hb.StartedAt > 0 || hb.Services != nil {
			var startedAt time.Time
			if hb.StartedAt > 0 {
				startedAt = time.Unix(hb.StartedAt, 0)
			}
			var services []storage.AgentService
			if hb.Services != nil {
				services = AgentServicesFromPayload(hb.Services)
			}
			agent.UpdatePresence(startedAt, services)
			if s.db != nil {
				if err := s.db.UpdateAgentPresence(agent.ID, startedAt, services); err != nil {
					log.Printf("Agent %s presence update failed: %v", agent.ID, err)
				}
			}
		}
	}

	// 发送 ACK
	resp := protocol.NewMessage(protocol.MessageTypeHeartbeat, map[string]interface{}{
		"status":     "ack",
		"serverTime": time.Now().Unix(),
	})
	resp.ID = msg.ID // 关联请求ID

	if err := agent.SendMessage(resp); err != nil {
		log.Printf("Agent %s heartbeat ack: %v", agent.ID, err)
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
