package rpc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Job Provider：控制面的统一执行通道（设计见 docs/guide/jobs-design.md）。
//
// 当前仅含 exec：在 Agent 本机带超时执行一条命令，返回退出码与输出尾部
// （截断上限与 server 侧 Job 行列对齐）。非零退出码与执行失败均以业务
// 字段形式返回（exit_code / error），RPC 层不视为协议错误——server 据此
// 落 job 状态（exit 0 → success，否则 failed，输出仍带回展示）。
type JobProvider struct{}

// NewJobProvider 创建 Job Provider（exec 平台无关，无条件注册）
func NewJobProvider() *JobProvider { return &JobProvider{} }

func (p *JobProvider) Type() string { return "job" }

// Call 分发 <job>.<action>；当前仅 exec
func (p *JobProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "exec":
		return p.Exec(params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

const (
	// jobExecMaxTimeout 命令超时上限（秒），防误配把 agent 挂死
	jobExecMaxTimeout = 300
	// jobExecMaxCommand 命令长度上限
	jobExecMaxCommand = 16 * 1024
	// jobExecMaxOutput 输出保留上限：只留尾部（与 storage.JobMaxOutput 对齐）
	jobExecMaxOutput = 64 * 1024
)

// ExecParams 入参：command（必填）、timeout_s（可选，缺省 60，上限
// jobExecMaxTimeout）。其余字段忽略（保留给后续 job.* 动作）。
type ExecParams struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s"`
}

// Exec 执行结果：exit_code（-1 = 启动失败/超时被杀）、output（截断尾部）、
// truncated、error（异常说明，成功为空）、duration_ms
func (p *JobProvider) Exec(params map[string]interface{}) (interface{}, error) {
	var ep ExecParams
	if v, ok := params["command"].(string); ok {
		ep.Command = v
	}
	// timeout_s 经 RPC JSON 解码为 float64；直调测试传 int。两者都收
	switch v := params["timeout_s"].(type) {
	case float64:
		if v > 0 {
			ep.TimeoutS = int(v)
		}
	case int:
		if v > 0 {
			ep.TimeoutS = v
		}
	}

	ep.Command = strings.TrimSpace(ep.Command)
	if ep.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if len(ep.Command) > jobExecMaxCommand {
		return nil, fmt.Errorf("command too large (max %d bytes)", jobExecMaxCommand)
	}
	if ep.TimeoutS > jobExecMaxTimeout {
		return nil, fmt.Errorf("timeout too large (max %ds)", jobExecMaxTimeout)
	}
	if ep.TimeoutS == 0 {
		ep.TimeoutS = 60
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ep.TimeoutS)*time.Second)
	defer cancel()

	cmd := newJobCmd(ctx, ep.Command)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	// 截断到尾部（保留末尾 64KB——错误信息通常在尾部）
	truncated := false
	if len(out) > jobExecMaxOutput {
		out = out[len(out)-jobExecMaxOutput:]
		truncated = true
	}

	res := map[string]interface{}{
		"exit_code":  -1,
		"output":     string(out),
		"truncated":  truncated,
		"error":      "",
		"duration_ms": duration.Milliseconds(),
	}
	if err != nil {
		exitCode := -1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		res["exit_code"] = exitCode
		switch {
		case ctx.Err() == context.DeadlineExceeded:
			res["error"] = fmt.Sprintf("timed out after %ds", ep.TimeoutS)
		default:
			res["error"] = trimExecErr(err.Error())
		}
		return res, nil
	}
	// 正常结束时 CombinedOutput 可能为空；退出码 0
	res["exit_code"] = 0
	return res, nil
}

// trimExecErr 错误串瘦身（exec 错误可能带完整命令原文与路径噪音）
func trimExecErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}