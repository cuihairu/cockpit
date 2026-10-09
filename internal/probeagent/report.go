package probeagent

import (
	"encoding/json"
	"os"

	"github.com/cuihairu/cockpit/core/healthprobe"
)

// Alerter 告警出口（herald 留位，简档 §4.4）：状态定性迁移即调，同步
// 语义；推送通道对接另批实现。Alert 在探测 goroutine 上被 Guard 锁内
// 回调链同步调用（core/healthprobe/state.go 契约），实现不得同步回读
// Agent/Guard 状态——会自死锁；需要快照时经 channel 交异步 goroutine。
type Alerter interface {
	Alert(tr healthprobe.Transition)
}

// NopAlerter 缺省空出口。
type NopAlerter struct{}

// Alert 空实现。
func (NopAlerter) Alert(healthprobe.Transition) {}

// WriteStatusFile 观测快照落 JSON（-status-file，简档 §4.1）：tmp+rename
// 原子替换，读侧永远看到完整文件。
func WriteStatusFile(path string, snaps []healthprobe.Snapshot) error {
	b, err := json.MarshalIndent(snaps, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
