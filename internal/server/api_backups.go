package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 备份管理 API（设计见 docs/guide/backup-design.md）。
//
//	GET    /api/backups/configs                 配置列表
//	POST   /api/backups/configs                 创建配置（审计）
//	PUT    /api/backups/configs/{id}            更新配置（审计）
//	DELETE /api/backups/configs/{id}            删除配置，级联删历史（审计）
//	POST   /api/backups/configs/{id}/run        立即运行一次（审计）
//	GET    /api/backups/configs/{id}/runs       该配置运行历史
//	GET    /api/backups/configs/{id}/files      浏览 agent 备份目录产物
//	POST   /api/backups/configs/{id}/files/delete  删除单个备份文件（审计）
//	GET    /api/backups/runs                    全部运行历史
const backupsAPIPrefix = "/api/backups/"

func (s *Server) registerBackupsAPI(mux *http.ServeMux) {
	mux.HandleFunc(backupsAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		s.authService().Middleware(s.handleBackupsAPI)(w, r)
	})
}

func (s *Server) handleBackupsAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, backupsAPIPrefix)
	parts := strings.Split(path, "/")

	switch {
	case len(parts) == 1 && parts[0] == "configs" && r.Method == http.MethodGet:
		s.handleBackupConfigsList(w, r)
	case len(parts) == 1 && parts[0] == "configs" && r.Method == http.MethodPost:
		s.handleBackupConfigCreate(w, r)
	case len(parts) == 1 && parts[0] == "runs" && r.Method == http.MethodGet:
		s.handleBackupRunsList(w, r)
	case len(parts) == 2 && parts[0] == "configs":
		id, err := strconv.Atoi(parts[1])
		if err != nil || id <= 0 {
			s.handleError(w, r, http.StatusBadRequest, "invalid config id")
			return
		}
		switch r.Method {
		case http.MethodPut:
			s.handleBackupConfigUpdate(w, r, uint(id))
		case http.MethodDelete:
			s.handleBackupConfigDelete(w, r, uint(id))
		default:
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	case len(parts) == 3 && parts[0] == "configs" && parts[2] == "run" && r.Method == http.MethodPost:
		s.withBackupConfig(w, r, parts[1], s.handleBackupRun)
	case len(parts) == 3 && parts[0] == "configs" && parts[2] == "runs" && r.Method == http.MethodGet:
		s.withBackupConfig(w, r, parts[1], s.handleBackupConfigRuns)
	case len(parts) == 3 && parts[0] == "configs" && parts[2] == "files" && r.Method == http.MethodGet:
		s.withBackupConfig(w, r, parts[1], s.handleBackupFiles)
	case len(parts) == 4 && parts[0] == "configs" && parts[2] == "files" && parts[3] == "delete" && r.Method == http.MethodPost:
		s.withBackupConfig(w, r, parts[1], s.handleBackupFileDelete)
	case len(parts) == 4 && parts[0] == "configs" && parts[2] == "files" && parts[3] == "sync-remote" && r.Method == http.MethodPost:
		s.withBackupConfig(w, r, parts[1], s.handleBackupFileSyncRemote)
	case len(parts) == 4 && parts[0] == "configs" && parts[2] == "files" && parts[3] == "download" && r.Method == http.MethodGet:
		s.withBackupConfig(w, r, parts[1], s.handleBackupFileDownload)
	case len(parts) == 4 && parts[0] == "configs" && parts[2] == "tasks" && r.Method == http.MethodGet:
		s.withBackupConfig(w, r, parts[1], func(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
			s.handleBackupTaskGet(w, r, cfg, parts[3])
		})
	case len(parts) == 3 && parts[0] == "configs" && parts[2] == "restore" && r.Method == http.MethodPost:
		s.withBackupConfig(w, r, parts[1], s.handleBackupRestore)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// withBackupConfig 解析 id 参数、加载配置后执行 handler
func (s *Server) withBackupConfig(w http.ResponseWriter, r *http.Request, idStr string, h func(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig)) {
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		s.handleError(w, r, http.StatusBadRequest, "invalid config id")
		return
	}
	cfg, err := s.db.GetBackupConfig(uint(id))
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "backup config not found")
		return
	}
	h(w, r, cfg)
}

// backupConfigView 配置视图（Sources 解析为数组）
type backupConfigView struct {
	ID         uint     `json:"id"`
	AgentID    string   `json:"agent_id"`
	Name       string   `json:"name"`
	Sources    []string `json:"sources"`
	DestDir    string   `json:"dest_dir"`
	Schedule   string   `json:"schedule"`
	Retention  int      `json:"retention"`
	RemoteDest string   `json:"remote_dest"`
	Enabled    bool     `json:"enabled"`
	LastRunAt  int64    `json:"last_run_at"`
	NextRunAt  int64    `json:"next_run_at"`
	LastStatus string   `json:"last_status"`
	CreatedAt  int64    `json:"created_at"`
}

func backupConfigToView(cfg *storage.BackupConfig) backupConfigView {
	return backupConfigView{
		ID:         cfg.ID,
		AgentID:    cfg.AgentID,
		Name:       cfg.Name,
		Sources:    ParseBackupSources(cfg.Sources),
		DestDir:    cfg.DestDir,
		Schedule:   cfg.Schedule,
		Retention:  cfg.Retention,
		RemoteDest: cfg.RemoteDest,
		Enabled:    cfg.Enabled,
		LastRunAt:  cfg.LastRunAt,
		NextRunAt:  cfg.NextRunAt,
		LastStatus: cfg.LastStatus,
		CreatedAt:  cfg.CreatedAt.Unix(),
	}
}

// backupConfigRequest 创建/更新请求体
type backupConfigRequest struct {
	AgentID    string   `json:"agent_id"`
	Name       string   `json:"name"`
	Sources    []string `json:"sources"`
	DestDir    string   `json:"dest_dir"`
	Schedule   string   `json:"schedule"`
	Retention  *int     `json:"retention"`
	RemoteDest string   `json:"remote_dest"` // rclone 远端目标，空=不启用异地（M2 D20）
	Enabled    *bool    `json:"enabled"`
}

// validateBackupConfigRequest 校验请求体（双端防御：agent 侧还有一道）
func (req *backupConfigRequest) validate() (string, bool) {
	if req.AgentID == "" {
		return "agent_id required", false
	}
	if !ValidBackupName(req.Name) {
		return "invalid name (lowercase letters, digits, - or _, max 64 chars)", false
	}
	if !ValidBackupSources(req.Sources) {
		return "sources must be a non-empty list of absolute paths", false
	}
	if req.DestDir == "" || !strings.HasPrefix(req.DestDir, "/") {
		return "dest_dir must be an absolute path", false
	}
	if !ValidBackupSchedule(req.Schedule) {
		return "schedule must be manual, daily@HH:mm or every:Nh (1-168)", false
	}
	if req.RemoteDest != "" && !ValidBackupRemoteDest(req.RemoteDest) {
		return "remote_dest must be rclone remote:path (e.g. my-s3:cockpit/backups)", false
	}
	return "", true
}

// requireAgentRclone 配置启用异地时校验目标 agent 有 rclone（M2 D23：
// 配置期拦截而非运行期才炸）
func (s *Server) requireAgentRclone(w http.ResponseWriter, r *http.Request, agentID string) bool {
	agent, ok := s.registry.Get(agentID)
	if !ok {
		return true // agent 不存在的报错由调用方处理
	}
	cap := agent.GetCapability("backup")
	enabled, _ := cap.Metadata["rclone"].(bool)
	if !enabled {
		s.handleError(w, r, http.StatusBadRequest,
			"remote_dest requires rclone installed on the agent host (rclone not found)")
		return false
	}
	return true
}

func (s *Server) handleBackupConfigsList(w http.ResponseWriter, r *http.Request) {
	configs, err := s.db.ListBackupConfigs()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list backup configs")
		return
	}
	views := make([]backupConfigView, 0, len(configs))
	for _, cfg := range configs {
		views = append(views, backupConfigToView(cfg))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"configs": views})
}

func (s *Server) handleBackupConfigCreate(w http.ResponseWriter, r *http.Request) {
	var req backupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if msg, ok := req.validate(); !ok {
		s.handleError(w, r, http.StatusBadRequest, msg)
		return
	}
	if _, ok := s.registry.Get(req.AgentID); !ok {
		s.handleError(w, r, http.StatusBadRequest, "agent not found")
		return
	}
	if req.RemoteDest != "" && !s.requireAgentRclone(w, r, req.AgentID) {
		return
	}
	retention := 0
	if req.Retention != nil {
		retention = *req.Retention
	}
	if retention < 0 || retention > backupMaxRetention {
		s.handleError(w, r, http.StatusBadRequest, "retention must be 0-365")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	now := time.Now()
	cfg := &storage.BackupConfig{
		AgentID:    req.AgentID,
		Name:       req.Name,
		Sources:    marshalSources(req.Sources),
		DestDir:    req.DestDir,
		Schedule:   req.Schedule,
		Retention:  retention,
		RemoteDest: req.RemoteDest,
		Enabled:    enabled,
		NextRunAt:  NextBackupRunAt(req.Schedule, now),
	}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create backup config")
		return
	}
	s.auditBackup(r, audit.ActionCreate, cfg.ID, cfg.Name, map[string]interface{}{"agent": cfg.AgentID, "schedule": cfg.Schedule})
	s.writeJSON(w, http.StatusCreated, backupConfigToView(cfg))
}

func (s *Server) handleBackupConfigUpdate(w http.ResponseWriter, r *http.Request, id uint) {
	cfg, err := s.db.GetBackupConfig(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "backup config not found")
		return
	}
	var req backupConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if msg, ok := req.validate(); !ok {
		s.handleError(w, r, http.StatusBadRequest, msg)
		return
	}
	if _, ok := s.registry.Get(req.AgentID); !ok {
		s.handleError(w, r, http.StatusBadRequest, "agent not found")
		return
	}
	retention := cfg.Retention
	if req.Retention != nil {
		retention = *req.Retention
	}
	if retention < 0 || retention > backupMaxRetention {
		s.handleError(w, r, http.StatusBadRequest, "retention must be 0-365")
		return
	}
	enabled := cfg.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 运行中禁止改动，避免终态回填与编辑互相踩
	if cfg.LastStatus == "running" {
		s.handleError(w, r, http.StatusConflict, "backup is running")
		return
	}
	if req.RemoteDest != "" && !s.requireAgentRclone(w, r, req.AgentID) {
		return
	}

	cfg.AgentID = req.AgentID
	cfg.Name = req.Name
	cfg.Sources = marshalSources(req.Sources)
	cfg.DestDir = req.DestDir
	cfg.Schedule = req.Schedule
	cfg.Retention = retention
	cfg.RemoteDest = req.RemoteDest
	cfg.Enabled = enabled
	cfg.NextRunAt = NextBackupRunAt(req.Schedule, time.Now())
	if err := s.db.UpdateBackupConfig(cfg); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to update backup config")
		return
	}
	s.auditBackup(r, audit.ActionUpdate, cfg.ID, cfg.Name, map[string]interface{}{"schedule": cfg.Schedule, "enabled": cfg.Enabled})
	s.writeJSON(w, http.StatusOK, backupConfigToView(cfg))
}

func (s *Server) handleBackupConfigDelete(w http.ResponseWriter, r *http.Request, id uint) {
	cfg, err := s.db.GetBackupConfig(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "backup config not found")
		return
	}
	if err := s.db.DeleteBackupConfig(id); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to delete backup config")
		return
	}
	s.auditBackup(r, audit.ActionDelete, id, cfg.Name, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	if cfg.LastStatus == "running" {
		s.handleError(w, r, http.StatusConflict, "backup is already running")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	// 手动运行不打乱调度节奏：不重算 NextRunAt（调度侧独立推进）
	if _, err := s.startBackupRun(cfg, "manual"); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.auditBackup(r, audit.ActionBackupRun, cfg.ID, cfg.Name, nil)
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleBackupConfigRuns(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	s.listRuns(w, r, cfg.ID)
}

func (s *Server) handleBackupRunsList(w http.ResponseWriter, r *http.Request) {
	s.listRuns(w, r, 0)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request, configID uint) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	runs, err := s.db.ListBackupRuns(configID, limit)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list backup runs")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

// handleBackupFiles 浏览 agent 备份目录产物（dir 取自配置，不接受客户端传目录）
func (s *Server) handleBackupFiles(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(cfg.AgentID, "backup.list", map[string]interface{}{"dir": cfg.DestDir})
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "agent returned error")
		return
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	files, _ := data["files"].([]interface{})
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

// handleBackupFileDelete 删除单个备份文件（文件名走双端正则校验）
func (s *Server) handleBackupFileDelete(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		s.handleError(w, r, http.StatusBadRequest, "name required")
		return
	}
	if !backupFileNameRe.MatchString(req.Name) {
		s.handleError(w, r, http.StatusBadRequest, "invalid file name")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(cfg.AgentID, "backup.delete", map[string]interface{}{
		"dir": cfg.DestDir, "name": req.Name,
	})
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "failed to delete file on agent")
		return
	}
	s.auditBackup(r, audit.ActionBackupDeleteFile, cfg.ID, cfg.Name, map[string]interface{}{"file": req.Name})
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleBackupFileSyncRemote 手动补传单个备份文件到远端（M2 D24：
// 自动推送失败通知后的面板内动作闭环；rclone 幂等，重复调用安全）
func (s *Server) handleBackupFileSyncRemote(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		s.handleError(w, r, http.StatusBadRequest, "name required")
		return
	}
	if !backupFileNameRe.MatchString(req.Name) {
		s.handleError(w, r, http.StatusBadRequest, "invalid file name")
		return
	}
	if cfg.RemoteDest == "" {
		s.handleError(w, r, http.StatusBadRequest, "remote_dest not configured on this backup")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(cfg.AgentID, "backup.remote.sync", map[string]interface{}{
		"dir": cfg.DestDir, "name": req.Name, "remoteDest": cfg.RemoteDest,
	})
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to sync file to remote")
		return
	}
	if rpcResp.Status == "error" {
		// agent 侧错误含 rclone stderr 摘要，透传给用户
		msg := rpcResp.Error
		if msg == "" {
			msg = "failed to sync file to remote"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}
	s.auditBackup(r, audit.ActionBackupRemoteSync, cfg.ID, cfg.Name, map[string]interface{}{"file": req.Name, "remoteDest": cfg.RemoteDest})
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

// handleBackupRestore 恢复备份到独立目录（M1.5，见 backup-design.md D9-D11）。
// 双确认：confirmName 必须与 file 完全一致（GitHub 删仓库模式），误触无法触发。
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	var req struct {
		File        string `json:"file"`
		DestDir     string `json:"dest_dir"`
		ConfirmName string `json:"confirm_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !backupFileNameRe.MatchString(req.File) {
		s.handleError(w, r, http.StatusBadRequest, "invalid file name")
		return
	}
	if req.ConfirmName != req.File {
		s.handleError(w, r, http.StatusBadRequest, "confirm_name must match file name")
		return
	}
	destDir := filepath.Clean(req.DestDir)
	if !filepath.IsAbs(destDir) || destDir == string(filepath.Separator) {
		s.handleError(w, r, http.StatusBadRequest, "dest_dir must be an absolute path other than /")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(cfg.AgentID, "backup.restore", map[string]interface{}{
		"dir": cfg.DestDir, "name": req.File, "destDir": destDir,
	})
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "failed to start restore on agent")
		return
	}
	s.auditBackup(r, audit.ActionBackupRestore, cfg.ID, cfg.Name,
		map[string]interface{}{"file": req.File, "dest_dir": destDir})
	data, _ := rpcResp.Data.(map[string]interface{})
	s.writeJSON(w, http.StatusOK, data)
}

// backupTaskIDRe 任务 ID 是 agent 生成的受限字符串，转发前先收紧
var backupTaskIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// handleBackupTaskGet 转发 backup.task.get，供前端轮询 restore 任务状态与日志
// （restore 不落 BackupRun 表——那是备份运行的语义）
func (s *Server) handleBackupTaskGet(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig, taskID string) {
	if !backupTaskIDRe.MatchString(taskID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	resp, err := s.CallAgent(cfg.AgentID, "backup.task.get", map[string]interface{}{"taskId": taskID})
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "failed to query task on agent")
		return
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	s.writeJSON(w, http.StatusOK, data)
}

// backupDownloadChunk server 每次从 agent 拉的分块大小（agent 上限 1MB）
const backupDownloadChunk = 256 * 1024

// backupDownloadTimeout 下载总超时；大包慢盘也给足余量
const backupDownloadTimeout = 15 * time.Minute

// handleBackupFileDownload 分块拉取备份文件并流式转发给浏览器（M1.5 D12）。
// server 不落盘：每块 base64 解码后直接写 ResponseWriter；首块带回文件总大小
// 作 Content-Length，客户端中断即停。
func (s *Server) handleBackupFileDownload(w http.ResponseWriter, r *http.Request, cfg *storage.BackupConfig) {
	name := r.URL.Query().Get("name")
	if !backupFileNameRe.MatchString(name) {
		s.handleError(w, r, http.StatusBadRequest, "invalid file name")
		return
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupDownloadTimeout)
	defer cancel()

	readChunk := func(offset int64) (data []byte, total int64, eof bool, err error) {
		resp, err := s.CallAgent(cfg.AgentID, "backup.read", map[string]interface{}{
			"dir": cfg.DestDir, "name": name,
			"offset": float64(offset), "length": float64(backupDownloadChunk),
		})
		if err != nil {
			return nil, 0, false, err
		}
		rpcResp, err := protocol.DecodeRPCResponse(resp)
		if err != nil || rpcResp.Status == "error" {
			return nil, 0, false, fmt.Errorf("agent read error")
		}
		m, _ := rpcResp.Data.(map[string]interface{})
		b64, _ := m["data"].(string)
		data, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, 0, false, fmt.Errorf("bad chunk data")
		}
		if v, ok := m["size"].(float64); ok {
			total = int64(v)
		}
		eof, _ = m["eof"].(bool)
		return data, total, eof, nil
	}

	offset := int64(0)
	chunk, total, eof, err := readChunk(0)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to read backup file: "+err.Error())
		return
	}
	if total > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(total, 10))
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	for {
		select {
		case <-ctx.Done():
			return // 客户端断开或超时，静默截断
		default:
		}
		if _, err := w.Write(chunk); err != nil {
			return
		}
		if eof || len(chunk) == 0 {
			break
		}
		offset += int64(len(chunk))
		if chunk, _, eof, err = readChunk(offset); err != nil {
			return // 中途失败：Content-Length 不完整，客户端侧可感知异常
		}
	}
}

// ============ helpers ============

func marshalSources(sources []string) string {
	b, _ := json.Marshal(sources)
	return string(b)
}

// auditBackup 备份审计（路径属配置信息可记；备份文件内容从不入审计）
func (s *Server) auditBackup(r *http.Request, action string, configID uint, name string, details map[string]interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceBackup,
		strconv.FormatUint(uint64(configID), 10),
		details, s.getClientIP(r), r.UserAgent())
}
