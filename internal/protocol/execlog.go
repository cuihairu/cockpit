package protocol

// execlog 执行留痕 wire 契约（agent-core 架构 §2 能力 8）：agent 侧每次
// 执行（service/job/file/backup…）完成即产生一条 ExecLogEntry，攒批随
// exec_log 消息上行；server 归集落审计表（username=cockpit-agent）。
// 类型放 protocol 是 wire 契约一份的原则——构造（agent）与归集（server）
// 共用同一结构，不做两侧各自为政的 map 拼装。

// ExecLogEntry 单次执行留痕。
type ExecLogEntry struct {
	// ID agent 生成的执行唯一标识（环内去重/排障对账用）。
	ID string `json:"id"`
	// Time 执行完成时刻（Unix 秒）。
	Time int64 `json:"time"`
	// Provider 执行面（service/job/file/backup…），落审计 Resource 列。
	Provider string `json:"provider"`
	// Action 动作（restart/write/delete/run…）。
	Action string `json:"action"`
	// Target 目标（unit/路径/任务 ID…）；空时 server 回落 execId。
	Target string `json:"target,omitempty"`
	// Status success / failure。
	Status string `json:"status"`
	// Error 失败摘要（成功为空；agent 侧负责截断）。
	Error string `json:"error,omitempty"`
	// DurationMs 执行耗时。
	DurationMs int64 `json:"durationMs,omitempty"`
	// Params 入参摘要（调用方脱敏后；凭据类入参不得进入）。
	Params map[string]string `json:"params,omitempty"`
}

// 执行状态取值（对齐 server 审计 Status 列的 success/failure）。
const (
	ExecStatusSuccess = "success"
	ExecStatusFailure = "failure"
)

// ExecLogPayload exec_log 消息负载：一批留痕。空批合法（no-op）。
type ExecLogPayload struct {
	Entries []ExecLogEntry `json:"entries"`
}

// DecodeExecLog 解析执行留痕上报。
func DecodeExecLog(msg *Message) (ExecLogPayload, error) {
	return DecodePayload[ExecLogPayload](msg)
}
