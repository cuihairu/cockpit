package detector

import (
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Detector 能力检测器接口
type Detector interface {
	// Name 检测器名称
	Name() string

	// Detect 检测是否具备该能力
	// 返回: 能力信息, 错误(检测失败时), nil(不具备该能力但不报错)
	Detect() (*protocol.Capability, error)

	// Priority 检测优先级（数字越小越先执行）
	Priority() int
}

// detectors 注册的所有检测器
var detectors = []Detector{}

// Register 注册检测器
func Register(d Detector) {
	detectors = append(detectors, d)
}

// All 获取所有注册的检测器
func All() []Detector {
	return detectors
}
