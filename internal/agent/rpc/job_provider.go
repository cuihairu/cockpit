package rpc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Job Provider：控制面的统一执行通道（设计见 docs/guide/jobs-design.md）。
//
// 当前仅含 exec：在 Agent 本机带超时执行一条命令，返回退出码与输出尾部
// （截断上限与 server 侧 Job 行列对齐）。非零退出码与执行失败均以业务
// 字段形式返回（exit_code / error），RPC 层不视为协议错误——server 据此
// 落 job 状态（exit 0 → success，否则 failed，输出仍带回展示）。
type JobProvider struct {
	mu       sync.Mutex             // guards sessions
	sessions map[string]jobSession  // jobId → ctx cancel；exec 期间持有，结束后注销
}

type jobSession struct {
	cancel context.CancelFunc
}

// NewJobProvider 创建 Job Provider（exec 平台无关，无条件注册）
func NewJobProvider() *JobProvider { return &JobProvider{sessions: make(map[string]jobSession)} }

func (p *JobProvider) Type() string { return "job" }

// Call 分发 <job>.<action>；当前含 exec、cancel（M2c）
func (p *JobProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "exec":
		return p.Exec(params)
	case "cancel":
		return p.Cancel(params)
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
	TimeoutS int  `json:"timeout_s"`
	// ID 是 job 唯一标识（server 下发 job.exec 时必带，用于 cancel 定位句柄）。
	// 旧版本 server 可能不传此字段；无 id 则不注册句柄表，cancel 幂等空操作。
	ID string `json:"id,omitempty"`
}

// Exec 执行结果：exit_code（-1 = 启动失败/超时被杀）、output（截断尾部）、
// truncated、error（异常说明，成功为空）、duration_ms
func (p *JobProvider) Exec(params map[string]interface{}) (interface{}, error) {
	var ep ExecParams
	if v, ok := params["command"].(string); ok {
		ep.Command = v
	}
	// timeout_s 经 RPC JSON 解码为 float64；直调测试传 int。两者都收
	if v, ok := params["timeout_s"].(float64); ok {
		if v > 0 {
			ep.TimeoutS = int(v)
		}
	} else if v, ok := params["timeout_s"].(int); ok {
		if v > 0 {
			ep.TimeoutS = v
		}
	}
	// id 由 server 提供；存在性非硬依赖（旧调用不含 id 也合法）
	if v, ok := params["id"].(string); ok {
		ep.ID = v
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

	// 按 id 注册句柄（取消信号查找用）；无 id 或注册冲突不阻塞主流程
	p.registerSession(ep.ID, cancel)

	cmd := newJobCmd(ctx, ep.Command)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	// 注销句柄（无论成功/失败/超时）。注意：注销前不再调用 cancel，cmd 已
	// 终结上下文；注销顺序 LIFO（注册先 → 注销先）
	p.unregisterSession(ep.ID)

	// 截断到尾部（保留末尾 64KB——错误信息通常在尾部）
	truncated := false
	if len(out) > jobExecMaxOutput {
		out = out[len(out)-jobExecMaxOutput:]
		truncated = true
	}

	res := map[string]interface{}{
		"exit_code": -1,
		"output":    string(out),
		"truncated": truncated,
		"error":     "",
		"duration_ms": duration.Milliseconds(),
	}
	if err != nil {
		exitCode := -1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		res["exit_code"] = exitCode
		switch {
		case ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled:
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

// Cancel 终止在途 job（C1-C5）：按 id 找到 ctx cancel 并触发之（进程组 SIGKILL）；
// 未知 id 返回成功空操作（幂等语义，C5）——让 server 无法区分「已停」与「早已结束」。
func (p *JobProvider) Cancel(params map[string]interface{}) (interface{}, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return map[string]interface{}{"cancelled": false}, nil
	}
	p.mu.Lock()
	sess, ok := p.sessions[id]
	p.mu.Unlock()
	if !ok {
		// 已注销（exec 自然结束）或未注册（无 id 的旧调用）；幂等成功
		return map[string]interface{}{"cancelled": false}, nil
	}
	sess.cancel()
	return map[string]interface{}{"cancelled": true}, nil
}

// registerSession 将 exec 的 ctx cancel 注入句柄表（线程安全）；id 为空或已存在即忽略。
func (p *JobProvider) registerSession(id string, cancel context.CancelFunc) {
	if id == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.sessions[id]; !exists {
		p.sessions[id] = jobSession{cancel: cancel}
	}
}

// unregisterSession 移除句柄表条目（exec 结束后调用）。
func (p *JobProvider) unregisterSession(id string) {
	if id == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, id)
}

// trimExecErr 错误串瘦身（exec 错误可能带完整命令原文与路径噪音）
func trimExecErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return s[:512] + "..."
	}
	return s
}
