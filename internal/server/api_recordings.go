package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 会话录制 API（见 docs/guide/recording-design.md）：
//
//	GET    /api/recordings                 列表（倒序，索引不含内容，不审计）
//	GET    /api/recordings/{sid}/cast      取 .cast 内容（回放/下载同源，记审计）
//	DELETE /api/recordings/{sid}           删除文件+元数据（记审计）
//
// 录制内容含历史终端输出（可能有敏感信息），取内容必须可追溯（D10）。

// handleRecordings /api/recordings 与 /api/recordings/{sid}[/cast] 分发
func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/recordings" {
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleRecordingsList(w, r)
		return
	}
	rest := strings.TrimPrefix(path, "/recordings/")
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[1] == "cast" {
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleRecordingCast(w, r, parts[0])
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleRecordingDelete(w, r, parts[0])
		return
	}
	s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
}

// handleRecordingsList 倒序列表（进行中也在列，duration=0）
func (s *Server) handleRecordingsList(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListTerminalRecordings(200)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list recordings")
		return
	}
	if list == nil {
		list = []*storage.TerminalRecording{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": list})
}

// handleRecordingCast 流式返回 .cast 文件（回放与下载同源）
func (s *Server) handleRecordingCast(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, err := s.db.GetTerminalRecording(sessionID); err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording not found")
		return
	}
	path := filepath.Join(s.recordingsDir(), sessionID+".cast")
	f, err := os.Open(path)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording file missing")
		return
	}
	defer f.Close()

	s.auditRecording(r, audit.ActionView, sessionID, nil)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sessionID+`.cast"`)
	io.Copy(w, f)
}

// handleRecordingDelete 删除录制文件与元数据
func (s *Server) handleRecordingDelete(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, err := s.db.GetTerminalRecording(sessionID); err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording not found")
		return
	}
	path := filepath.Join(s.recordingsDir(), sessionID+".cast")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		s.handleError(w, r, http.StatusInternalServerError, "failed to remove recording file")
		return
	}
	if err := s.db.DeleteTerminalRecording(sessionID); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to delete recording")
		return
	}
	s.auditRecording(r, audit.ActionDelete, sessionID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// auditRecording 记录制内容访问审计
func (s *Server) auditRecording(r *http.Request, action, sessionID string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceRecording, sessionID,
		details, s.getClientIP(r), r.UserAgent())
}
