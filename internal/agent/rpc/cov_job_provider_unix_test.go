//go:build unix

package rpc

import (
	"context"
	"testing"
)

// newJobCmd 的 Cancel 在进程未启动（Process 为 nil）时直接返回 nil——
// CommandContext 只在启动成功后才调 Cancel，该分支为纯守卫，直调覆盖
func TestJobCmdCancelBeforeStart(t *testing.T) {
	c := newJobCmd(context.Background(), "echo hi")
	if c.Process != nil {
		t.Fatalf("Process should be nil before start")
	}
	if err := c.Cancel(); err != nil {
		t.Fatalf("Cancel before start: %v", err)
	}
}