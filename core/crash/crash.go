// Package crash 崩溃采集留位（agent-core ① 通用能力层）：仅落事件模型与
// 上报契约占位，不实现采集——分级方案（panic 钩子/信号栈转储/核心转储脱敏
// 上传）按调研另批立项（对标 croupier crash-capture-survey 节奏）。事件模型
// 先行固定，避免后续批次翻转调用方契约。
package crash

import "time"

// Event 一次崩溃/异常退出事件（字段占位，采集实现批次补齐；与退出语义契约
// 对齐——优雅退出不产生本事件，崩溃/硬杀才产生）。
type Event struct {
	Kind       string    // panic / signal / watchdog …
	Message    string    // 一线错误信息
	Stack      []byte    // 调用栈快照（脱敏由实现批次负责）
	OccurredAt time.Time // 事件时刻
}

// Reporter 崩溃事件上报契约：本地留痕 + 统一上行的组装由实现批次决定，
// core 只定义方向（实现方可内部走 report.Upstream，core 不反向依赖）。
type Reporter interface {
	Report(Event)
}
