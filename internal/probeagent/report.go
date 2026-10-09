package probeagent

import (
	"encoding/json"
	"os"

	"github.com/cuihairu/cockpit/core/healthprobe"
)

// Alerter 告警出口（herald 留位，简档 §4.4）：状态定性迁移即调，同步
// 语义；推送通道对接另批实现。
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
