package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
)

// 会话录制 API（见 docs/guide/recording-design.md）：
//
//	GET    /api/recordings                 列表（倒序，索引不含内容，不审计）
//	GET    /api/recordings/config          配置读取（M2 D19：开关/保留/异地/rclone 探测）
//	PUT    /api/recordings/config          配置写入
//	GET    /api/recordings/{sid}/cast      取 .cast 内容（回放/下载同源，记审计）
//	POST   /api/recordings/{sid}/sync-remote   手动补推归档（M2 D18，记审计）
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
	if path == "/recordings/config" {
		s.handleRecordingsConfig(w, r)
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
	if len(parts) == 2 && parts[1] == "sync-remote" {
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleRecordingSyncRemote(w, r, parts[0])
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
	// gorm Find 空表恒返回非 nil 空 slice，无需兜底
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": list})
}

// handleRecordingCast 流式返回录制文件（回放与下载同源）。
// URL 沿用 /{sid}/cast（语义是「取录制内容」）；后缀按 rec.Format 分流
// （M3 D4：cast=asciinema 流、guac=Guacamole 会话流），前端零改动。
func (s *Server) handleRecordingCast(w http.ResponseWriter, r *http.Request, sessionID string) {
	rec, err := s.db.GetTerminalRecording(sessionID)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording not found")
		return
	}
	ext := recordingExt(rec.Format)
	path := filepath.Join(s.recordingsDir(), sessionID+ext)
	f, err := os.Open(path)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording file missing")
		return
	}
	defer f.Close()

	s.auditRecording(r, audit.ActionView, sessionID, nil)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sessionID+ext+`"`)
	io.Copy(w, f)
}

// handleRecordingDelete 删除录制文件与元数据
func (s *Server) handleRecordingDelete(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, err := s.db.GetTerminalRecording(sessionID); err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording not found")
		return
	}
	ext := ".cast"
	if rec, err := s.db.GetTerminalRecording(sessionID); err == nil && rec != nil {
		ext = recordingExt(rec.Format)
	}
	path := filepath.Join(s.recordingsDir(), sessionID+ext)
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

// handleRecordingsConfig 配置读取/写入（M2 D19：补 M1 缺口——两个 Setting
// 此前无 REST/UI 写入口；异地归档目标同 server 备份 M2 校验规则）
func (s *Server) handleRecordingsConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		enabled := s.recordingEnabled()
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":            enabled,
			"retention_days":     s.recordingRetentionDays(),
			"remote_dest":        s.recordingRemoteDest(),
			"rclone_available":   rcloneAvailable(),
			"max_retention_days": recordingMaxRetentionDays,
		})
	case http.MethodPut:
		var req struct {
			Enabled       *bool   `json:"enabled"`
			RetentionDays *int    `json:"retention_days"`
			RemoteDest    *string `json:"remote_dest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		if req.Enabled != nil {
			v := "false"
			if *req.Enabled {
				v = "true"
			}
			if err := s.db.SetSetting(RecordingEnabledSettingKey, v); err != nil {
				s.handleError(w, r, http.StatusInternalServerError, "failed to save enabled")
				return
			}
		}
		if req.RetentionDays != nil {
			if *req.RetentionDays < 0 || *req.RetentionDays > recordingMaxRetentionDays {
				s.handleError(w, r, http.StatusBadRequest,
					fmt.Sprintf("retention_days out of range [0, %d]", recordingMaxRetentionDays))
				return
			}
			if err := s.db.SetSetting(RecordingRetentionSettingKey, strconv.Itoa(*req.RetentionDays)); err != nil {
				s.handleError(w, r, http.StatusInternalServerError, "failed to save retention_days")
				return
			}
		}
		// M2 D14：写时严格校验（读时宽松不阻塞录制，写入口必须拦住脏值）
		if req.RemoteDest != nil {
			dest := strings.TrimSpace(*req.RemoteDest)
			if dest != "" && (len(dest) > serverBackupRemoteDestMaxLen ||
				!regexp.MustCompile(serverBackupRemoteDestRe).MatchString(dest)) {
				s.handleError(w, r, http.StatusBadRequest, "remote_dest must be remote:path (e.g. gdrive:recordings)")
				return
			}
			if err := s.db.SetSetting(RecordingRemoteDestSettingKey, dest); err != nil {
				s.handleError(w, r, http.StatusInternalServerError, "failed to save remote_dest")
				return
			}
		}
		s.auditRecording(r, audit.ActionUpdate, "config", nil)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":        s.recordingEnabled(),
			"retention_days": s.recordingRetentionDays(),
			"remote_dest":    s.recordingRemoteDest(),
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleRecordingSyncRemote 手动补推录制归档（M2 D18：收到失败通知后的
// 补救路径，rclone copy 幂等；同步执行同 server 备份补推）
func (s *Server) handleRecordingSyncRemote(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !recordingSessionIDRe.MatchString(sessionID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid session id")
		return
	}
	if s.recordingRemoteDest() == "" {
		s.handleError(w, r, http.StatusBadRequest, "remote_dest not configured")
		return
	}
	if _, err := s.db.GetTerminalRecording(sessionID); err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording not found")
		return
	}
	ext := ".cast"
	if rec, err := s.db.GetTerminalRecording(sessionID); err == nil && rec != nil {
		ext = recordingExt(rec.Format)
	}
	if _, err := os.Stat(filepath.Join(s.recordingsDir(), sessionID+ext)); err != nil {
		s.handleError(w, r, http.StatusNotFound, "recording file missing")
		return
	}
	s.auditRecording(r, audit.ActionUpdate, sessionID, map[string]string{"sync_remote": "true"})
	s.pushRecordingRemote(sessionID)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
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
