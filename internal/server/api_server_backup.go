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

// Server 自身数据库备份 API（见 docs/guide/server-backup-design.md）：
//
//	GET    /api/server-backups            列表（目录扫描，不审计）
//	GET    /api/server-backups/config     配置读取（M2 含 remote_dest/rclone_available）
//	PUT    /api/server-backups/config     配置写入（间隔/保留/异地目标）
//	POST   /api/server-backups/run        立即备份（记审计；remote_dest 非空时顺带推送）
//	POST   /api/server-backups/{name}/sync-remote   手动补推（M2 D17，记审计）
//	GET    /api/server-backups/{name}/download   下载（记审计）
//	DELETE /api/server-backups/{name}            删除（记审计）

// handleServerBackups 分发 /api/server-backups 及子路径
func (s *Server) handleServerBackups(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/server-backups"), "/")
	switch {
	case sub == "":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleServerBackupsList(w, r)
	case sub == "config":
		s.handleServerBackupsConfig(w, r)
	case sub == "run":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleServerBackupsRun(w, r)
	case strings.HasSuffix(sub, "/sync-remote"):
		name := strings.TrimSuffix(sub, "/sync-remote")
		if !isValidServerBackupName(name) {
			s.handleError(w, r, http.StatusBadRequest, "invalid backup name")
			return
		}
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleServerBackupSyncRemote(w, r, name)
	case strings.HasSuffix(sub, "/download"):
		name := strings.TrimSuffix(sub, "/download")
		if !isValidServerBackupName(name) {
			s.handleError(w, r, http.StatusBadRequest, "invalid backup name")
			return
		}
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleServerBackupDownload(w, r, name)
	default:
		// 剩余合法形态只有 {name}（DELETE 删除）
		if !isValidServerBackupName(sub) {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		if r.Method != http.MethodDelete {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleServerBackupDelete(w, r, sub)
	}
}

// handleServerBackupsList 备份列表（文件名倒序 = 最新在前）
func (s *Server) handleServerBackupsList(w http.ResponseWriter, r *http.Request) {
	list, err := s.listServerBackups()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list server backups")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"data": list})
}

// handleServerBackupsConfig 配置读取/写入
func (s *Server) handleServerBackupsConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"interval_hours":     s.serverBackupIntervalHours(),
			"retention_days":     s.serverBackupRetentionDays(),
			"remote_dest":        s.serverBackupRemoteDest(),
			"rclone_available":   rcloneAvailable(),
			"max_interval_hours": serverBackupMaxIntervalHours,
			"max_retention_days": serverBackupMaxRetentionDays,
		})
	case http.MethodPut:
		var req struct {
			IntervalHours *int    `json:"interval_hours"`
			RetentionDays *int    `json:"retention_days"`
			RemoteDest    *string `json:"remote_dest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
			return
		}
		validate := func(v *int, max int, key string) error {
			if v == nil {
				return nil
			}
			if *v < 0 || *v > max {
				return fmt.Errorf("%s out of range [0, %d]", key, max)
			}
			return s.db.SetSetting(key, strconv.Itoa(*v))
		}
		if err := validate(req.IntervalHours, serverBackupMaxIntervalHours, ServerBackupIntervalSettingKey); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if err := validate(req.RetentionDays, serverBackupMaxRetentionDays, ServerBackupRetentionSettingKey); err != nil {
			s.handleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		// M2 D13：写时严格校验（读时宽松不阻塞本地备份，写入口必须拦住脏值）
		if req.RemoteDest != nil {
			dest := strings.TrimSpace(*req.RemoteDest)
			if dest != "" && (len(dest) > serverBackupRemoteDestMaxLen ||
				!regexp.MustCompile(serverBackupRemoteDestRe).MatchString(dest)) {
				s.handleError(w, r, http.StatusBadRequest, "remote_dest must be remote:path (e.g. gdrive:cockpit)")
				return
			}
			if err := s.db.SetSetting(ServerBackupRemoteDestSettingKey, dest); err != nil {
				s.handleError(w, r, http.StatusInternalServerError, "failed to save remote_dest")
				return
			}
		}
		s.auditServerBackup(r, audit.ActionUpdate, "config", nil)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"interval_hours": s.serverBackupIntervalHours(),
			"retention_days": s.serverBackupRetentionDays(),
			"remote_dest":    s.serverBackupRemoteDest(),
		})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleServerBackupsRun 立即备份
func (s *Server) handleServerBackupsRun(w http.ResponseWriter, r *http.Request) {
	name, err := s.runServerBackup()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditServerBackup(r, audit.ActionCreate, name, nil)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"name": name})
}

// handleServerBackupSyncRemote 手动补推指定本地备份到当前 remote_dest
// （M2 D17：对称 agent sync-remote；收到失败通知后的补救路径，rclone 幂等）
func (s *Server) handleServerBackupSyncRemote(w http.ResponseWriter, r *http.Request, name string) {
	if s.serverBackupRemoteDest() == "" {
		s.handleError(w, r, http.StatusBadRequest, "remote_dest not configured")
		return
	}
	if _, err := os.Stat(filepath.Join(s.serverBackupDir(), name)); err != nil {
		s.handleError(w, r, http.StatusNotFound, "backup file not found")
		return
	}
	s.auditServerBackup(r, audit.ActionUpdate, name, map[string]string{"sync_remote": "true"})
	s.pushServerBackupRemote(name)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// handleServerBackupDownload 流式下载备份文件
func (s *Server) handleServerBackupDownload(w http.ResponseWriter, r *http.Request, name string) {
	path := filepath.Join(s.serverBackupDir(), name)
	f, err := os.Open(path)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "backup file not found")
		return
	}
	defer f.Close()

	s.auditServerBackup(r, audit.ActionExport, name, nil)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	io.Copy(w, f)
}

// handleServerBackupDelete 删除备份文件
func (s *Server) handleServerBackupDelete(w http.ResponseWriter, r *http.Request, name string) {
	path := filepath.Join(s.serverBackupDir(), name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			s.handleError(w, r, http.StatusNotFound, "backup file not found")
		} else {
			s.handleError(w, r, http.StatusInternalServerError, "failed to delete backup")
		}
		return
	}
	s.auditServerBackup(r, audit.ActionDelete, name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// auditServerBackup 记 server 备份审计（手动备份/配置变更/下载/删除）
func (s *Server) auditServerBackup(r *http.Request, action, name string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceServerBackup, name,
		details, s.getClientIP(r), r.UserAgent())
}
