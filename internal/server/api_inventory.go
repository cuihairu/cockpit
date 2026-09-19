package server

import (
	"net/http"
)

// handleInventoryConsistency GET /api/inventory/consistency（drift-design.md
// M6 D28/D30）：inventory 声明 vs agent 实报的按需读模型比对。浏览类不审计
// （同 drift check 口径）；inventory sync 未启用时 503 指名引导。
func (s *Server) handleInventoryConsistency(w http.ResponseWriter, r *http.Request) {
	if s.inventorySync == nil {
		s.handleError(w, r, http.StatusServiceUnavailable,
			"inventory sync 未启用：在 config.yaml 设置 inventory.path 指向 inventory YAML 后重启")
		return
	}
	report, err := s.inventorySync.Consistency()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, report)
}
