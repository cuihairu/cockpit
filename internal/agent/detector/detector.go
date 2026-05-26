package detector

import (
	"sync"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Detector 能力检测器接口
type Detector interface {
	Name() string
	Detect() (*protocol.Capability, error)
	Priority() int
}

var (
	detectors []Detector
	detMu     sync.RWMutex
)

func Register(d Detector) {
	detMu.Lock()
	defer detMu.Unlock()
	detectors = append(detectors, d)
}

func All() []Detector {
	detMu.RLock()
	defer detMu.RUnlock()
	out := make([]Detector, len(detectors))
	copy(out, detectors)
	return out
}
