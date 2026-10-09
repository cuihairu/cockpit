package healthprobe

import "time"

// windowCapacity 每 target 已关闭故障窗口保留上限：防长跑内存膨胀；
// 更久回查属存储层职责（server 侧），探针侧只保近期。
const windowCapacity = 64

// Window 一次服务不可用区间（故障窗口，croupier system 角色探针口径）：
// Healthy→Faulty 时开（StartedAt），Faulty→Healthy 时关（EndedAt）。
// EndedAt 零值表示仍在故障中。序列 [StartedAt, EndedAt] 即「什么时间段
// 服务不可用」的回查答案。
type Window struct {
	Target    string    `json:"target"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
	LastError string    `json:"lastError"`
}

// pushClosed 追加已关窗口并维持环形容量（丢最旧）。
func (g *Guard) pushClosed(w Window) {
	g.closed = append(g.closed, w)
	if len(g.closed) > windowCapacity {
		g.closed = g.closed[len(g.closed)-windowCapacity:]
	}
}
