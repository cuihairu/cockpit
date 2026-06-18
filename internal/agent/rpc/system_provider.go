package rpc

import (
	"fmt"
	"runtime"
	"time"
)

// ============ System Provider (内置) ============

type SystemProvider struct {
	startTime time.Time
}

func NewSystemProvider() *SystemProvider {
	return &SystemProvider{
		startTime: time.Now(),
	}
}

func (p *SystemProvider) Type() string { return "system" }

func (p *SystemProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "status":
		return p.Status(params)
	case "info":
		return p.Info(params)
	case "version":
		return map[string]interface{}{
			"version": "0.1.0",
			"build":   "dev",
		}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *SystemProvider) Status(params map[string]interface{}) (interface{}, error) {
	// 计算 Agent 运行时间
	uptime := time.Since(p.startTime)

	return map[string]interface{}{
		"status":     "ok",
		"uptime":     uptime.String(),
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}, nil
}

func (p *SystemProvider) Info(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"capabilities": []string{"pve", "docker", "openwrt"},
		"version":      "0.1.0",
	}, nil
}
