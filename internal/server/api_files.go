package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 远程文件管理 API（设计见 docs/guide/file-manager-design.md）。
// 挂在 /api/agents/{id}/files/... 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	POST /api/agents/{id}/files/list     {dir}
//	POST /api/agents/{id}/files/read     {path, offset, length}（编辑器专用）
//	POST /api/agents/{id}/files/write    {path, data, truncate}（审计）
//	POST /api/agents/{id}/files/mkdir    {path}（审计）
//	POST /api/agents/{id}/files/delete   {path}（审计）
//	POST /api/agents/{id}/files/rename   {path, name}（审计）
//	POST /api/agents/{id}/files/search   {dir, query, caseSensitive}（文本搜索，不审计）
//	GET  /api/agents/{id}/files/download?path=   分块流式转发
//
// 只记变更类操作的审计（D9）；文件内容从不入审计与日志。

// filesDownloadChunk 下载分块大小（agent 上限 1MB，与备份下载一致）
const filesDownloadChunk = 256 * 1024

// filesDownloadTimeout 下载总超时
const filesDownloadTimeout = 15 * time.Minute

// handleAgentFilesAPI 分发 /api/agents/{id}/files/{action}
// rest 是 "/agents/" 之后的部分（形如 "{agentID}/files/list"）
func (s *Server) handleAgentFilesAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/files/"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	action := rest[idx+len(suffix):]
	if agentID == "" || action == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if _, ok := s.registry.Get(agentID); !ok {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline")
		return
	}

	switch action {
	case "list", "read", "write", "mkdir", "delete", "rename", "search":
		if r.Method != http.MethodPost {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.forwardFileRPC(w, r, agentID, "file."+action, action)
	case "download":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleFileDownload(w, r, agentID)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// fileRPRequest 通用转发请求体：path 类 + read/write/search 专有字段
type fileRPRequest struct {
	Dir           string `json:"dir"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	Offset        *int64 `json:"offset"`
	Length        *int64 `json:"length"`
	Data          string `json:"data"`
	Truncate      *bool  `json:"truncate"`
	Query         string `json:"query"`
	CaseSensitive *bool  `json:"caseSensitive"`
}

// cleanServerPath server 侧路径校验，规则与 agent 的 cleanAbsPath 一致（双端防御）：
// Clean 后必须为绝对路径且不为根目录
func cleanServerPath(path, what string) (string, bool) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) || cleaned == string(filepath.Separator) {
		return "", false
	}
	return cleaned, true
}

// forwardFileRPC 校验路径后转发 RPC 并透传结果
func (s *Server) forwardFileRPC(w http.ResponseWriter, r *http.Request, agentID, method, action string) {
	var req fileRPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	params := map[string]interface{}{}
	switch action {
	case "list":
		dir, ok := cleanServerPath(req.Dir, "dir")
		if !ok {
			s.handleError(w, r, http.StatusBadRequest, "dir must be an absolute path")
			return
		}
		params["dir"] = dir
	case "read", "write", "mkdir", "delete":
		path, ok := cleanServerPath(req.Path, "path")
		if !ok {
			s.handleError(w, r, http.StatusBadRequest, "path must be an absolute path")
			return
		}
		params["path"] = path
	case "rename":
		path, ok := cleanServerPath(req.Path, "path")
		if !ok {
			s.handleError(w, r, http.StatusBadRequest, "path must be an absolute path")
			return
		}
		if !validFileName(req.Name) {
			s.handleError(w, r, http.StatusBadRequest, "invalid name")
			return
		}
		params["path"] = path
		params["name"] = req.Name
	case "search":
		// 双端同规则（D12）：dir 路径校验 + query 非空 ≤256；浏览性质不审计
		dir, ok := cleanServerPath(req.Dir, "dir")
		if !ok {
			s.handleError(w, r, http.StatusBadRequest, "dir must be an absolute path")
			return
		}
		if req.Query == "" || len(req.Query) > 256 {
			s.handleError(w, r, http.StatusBadRequest, "query must be 1-256 bytes")
			return
		}
		params["dir"] = dir
		params["query"] = req.Query
		if req.CaseSensitive != nil {
			params["caseSensitive"] = *req.CaseSensitive
		}
	}

	if action == "read" {
		offset, length := int64(0), int64(filesDownloadChunk)
		if req.Offset != nil {
			offset = *req.Offset
		}
		if req.Length != nil && *req.Length > 0 {
			length = *req.Length
		}
		params["offset"] = float64(offset)
		params["length"] = float64(length)
	}
	if action == "write" {
		raw, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			s.handleError(w, r, http.StatusBadRequest, "data must be base64")
			return
		}
		// 与 agent 上限一致的上限预检（默认拒绝超大请求体打穿内存）
		if len(raw) > 1024*1024 {
			s.handleError(w, r, http.StatusBadRequest, "chunk too large (max 1MB)")
			return
		}
		truncate := true
		if req.Truncate != nil {
			truncate = *req.Truncate
		}
		params["data"] = req.Data
		params["truncate"] = truncate
	}

	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil || rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, "agent rejected the operation")
		return
	}
	data, _ := rpcResp.Data.(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}

	switch action {
	case "write":
		// 审计只记新写入会话（truncate=true：编辑保存/分块首块/覆盖上传）；
		// append 续块（分块上传的 truncate=false）不记——否则大文件分块上传
		// 每块一条 file_write 刷屏（file-manager-design M3/D18）
		if req.Truncate == nil || *req.Truncate {
			s.auditFile(r, agentID, audit.ActionFileWrite, req.Path, nil)
		}
	case "mkdir":
		s.auditFile(r, agentID, audit.ActionFileMkdir, req.Path, nil)
	case "delete":
		s.auditFile(r, agentID, audit.ActionFileDelete, req.Path, nil)
	case "rename":
		s.auditFile(r, agentID, audit.ActionFileRename, req.Path,
			map[string]interface{}{"new_name": req.Name})
	}
	s.writeJSON(w, http.StatusOK, data)
}

// validFileName rename 目标：纯文件名，不含路径分隔符（两种平台写法都拒）
func validFileName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, "/\\") && !strings.ContainsRune(name, 0)
}

// handleFileDownload 分块拉取文件流式转发（备份下载同款骨架，server 不落盘）
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request, agentID string) {
	rawPath := r.URL.Query().Get("path")
	cleaned, ok := cleanServerPath(rawPath, "path")
	if !ok {
		s.handleError(w, r, http.StatusBadRequest, "path must be an absolute path")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), filesDownloadTimeout)
	defer cancel()

	name := filepath.Base(cleaned)
	readChunk := func(offset int64) (data []byte, total int64, eof bool, err error) {
		resp, err := s.CallAgent(agentID, "file.read", map[string]interface{}{
			"path": cleaned, "offset": float64(offset), "length": float64(filesDownloadChunk),
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
		s.handleError(w, r, http.StatusBadGateway, "failed to read file: "+err.Error())
		return
	}
	if total > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(total, 10))
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	for {
		select {
		case <-ctx.Done():
			return // 客户端断开或超时
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
			return // 中途失败：Content-Length 不完整，客户端可感知异常
		}
	}
}

// auditFile 文件变更审计（路径属操作信息可记；内容从不入审计）
func (s *Server) auditFile(r *http.Request, agentID, action, path string, details map[string]interface{}) {
	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	if details == nil {
		details = map[string]interface{}{}
	}
	details["agent"] = agentID
	s.audit.LogResource(username, action, audit.ResourceFile, path,
		details, s.getClientIP(r), r.UserAgent())
}
