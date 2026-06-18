package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// handleSystemInfo 解析心跳中的 systemInfo 并落库（历史指标 + 快照）
func (s *Server) handleSystemInfo(agentID string, info *protocol.SystemInfoPayload) {
	if info == nil {
		return
	}
	now := time.Now()

	// 类型化字段直接落库，消除手动断言
	metric := &storage.SystemMetric{
		AgentID:          agentID,
		Timestamp:        now,
		CPUUsage:         info.CPUUsage,
		CPUCores:         info.CPUCores,
		CPUFreqMHz:       info.CPUFreqMHz,
		MemTotal:         info.MemTotal,
		MemUsed:          info.MemUsed,
		MemAvailable:     info.MemAvailable,
		MemUsagePercent:  info.MemUsagePercent,
		DiskTotal:        info.DiskTotal,
		DiskUsed:         info.DiskUsed,
		DiskFree:         info.DiskFree,
		DiskUsagePercent: info.DiskUsagePercent,
		NetBytesSent:     info.NetBytesSent,
		NetBytesRecv:     info.NetBytesRecv,
		OSName:           info.OSName,
		OSVersion:        info.OSVersion,
		Arch:             info.Arch,
		Uptime:           info.Uptime,
		Load1:            info.Load1,
		Load5:            info.Load5,
		Load15:           info.Load15,
		CreatedAt:        now,
	}

	if err := s.db.SaveSystemMetric(metric); err != nil {
		log.Printf("Failed to save system metric for agent %s: %v", agentID, err)
	}

	// 更新快照（去重，与 metric 同源）
	snapshot := &storage.SystemInfoSnapshot{
		AgentID:          metric.AgentID,
		CPUUsage:         metric.CPUUsage,
		CPUCores:         metric.CPUCores,
		CPUFreqMHz:       metric.CPUFreqMHz,
		MemTotal:         metric.MemTotal,
		MemUsed:          metric.MemUsed,
		MemAvailable:     metric.MemAvailable,
		MemUsagePercent:  metric.MemUsagePercent,
		DiskTotal:        metric.DiskTotal,
		DiskUsed:         metric.DiskUsed,
		DiskFree:         metric.DiskFree,
		DiskUsagePercent: metric.DiskUsagePercent,
		NetBytesSent:     metric.NetBytesSent,
		NetBytesRecv:     metric.NetBytesRecv,
		OSName:           metric.OSName,
		OSVersion:        metric.OSVersion,
		Arch:             metric.Arch,
		Uptime:           metric.Uptime,
		Load1:            metric.Load1,
		Load5:            metric.Load5,
		Load15:           metric.Load15,
		Hostname:         info.Hostname,
		UpdatedAt:        now,
	}

	if err := s.db.UpdateSystemInfoSnapshot(snapshot); err != nil {
		log.Printf("Failed to update system info snapshot for agent %s: %v", agentID, err)
	}
}
