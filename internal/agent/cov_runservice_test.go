package agent

// cov_runservice_test.go RunService（Windows 服务的 SCM 控制路径）在 linux
// 侧可验证的语义：参数校验透传 + stop 通道先关时优雅退出返回 nil
//（与 cov_start_signal_test.go 的信号路径同口径，服务路径由 CI 真 Windows
// 走查覆盖）。

import (
	"strings"
	"testing"
)

func TestRunServiceValidateFail(t *testing.T) {
	cmd := &StartCmd{}
	if err := cmd.RunService(make(chan struct{})); err == nil ||
		!strings.Contains(err.Error(), "missing required -server flag") {
		t.Fatalf("want missing -server error, got %v", err)
	}
}

func TestRunServiceNilWhenStoppedBeforeStart(t *testing.T) {
	cmd := &StartCmd{Server: "ws://127.0.0.1:1/unreachable"}
	stop := make(chan struct{})
	close(stop) // SCM Stop 先于 agent 启动到达：退出而非报错
	if err := cmd.RunService(stop); err != nil {
		t.Fatalf("RunService with pre-closed stop should return nil, got %v", err)
	}
}
