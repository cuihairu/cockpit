package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Agent 升级与回滚 API（P0 Agent 第四片，见 docs/design/agent-upgrade-rollback.md）。
// 边界（设计 §2）：**不做批量升级**——升级 API 只接受单 agent 显式指定；
// agent 二进制 OS 级换装需真机验收（诚实延后，本片只落控制面）。

// handleUpgradeAPI 分发 /upgrade 面：
//
//	GET  /upgrade             升级意图列表（?agentId= 过滤）
//	GET  /upgrade/{id}        单条意图（终态与原因，供排障）
//	GET  /upgrade/artifacts   产物登记列表
//	POST /upgrade/artifacts   上传产物（multipart：file + version/arch/os/min_version）
func (s *Server) handleUpgradeAPI(w http.ResponseWriter, r *http.Request, path string) {
	rest := strings.TrimPrefix(path, "/upgrade")
	rest = strings.TrimPrefix(rest, "/")

	switch {
	case rest == "":
		s.handleUpgradeList(w, r)
	case rest == "artifacts":
		if r.Method == http.MethodPost {
			s.handleArtifactUpload(w, r)
			return
		}
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		arts, err := s.db.ListArtifacts()
		if err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "failed to list artifacts")
			return
		}
		if arts == nil {
			arts = []storage.AgentArtifact{}
		}
		s.writeJSON(w, http.StatusOK, arts)
	case rest != "":
		s.handleUpgradeGet(w, r, rest)
	}
}

// handleUpgradeList 升级意图列表（agentId 空 = 全部）。
func (s *Server) handleUpgradeList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	intents, err := s.db.ListUpgrades(r.URL.Query().Get("agentId"), 0)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list upgrades")
		return
	}
	if intents == nil {
		intents = []storage.AgentUpgrade{}
	}
	s.writeJSON(w, http.StatusOK, intents)
}

// handleUpgradeGet 单条升级意图。
func (s *Server) handleUpgradeGet(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u, err := s.db.GetUpgrade(id)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to load upgrade intent")
		return
	}
	if u == nil {
		s.handleError(w, r, http.StatusNotFound, "upgrade intent not found")
		return
	}
	s.writeJSON(w, http.StatusOK, u)
}

// handleArtifactUpload 上传升级产物（multipart）。sha256 由服务器对内容计算，
// 不接受客户端声明的哈希（校验产物内容一致性，见设计 §3.1）。
func (s *Server) handleArtifactUpload(w http.ResponseWriter, r *http.Request) {
	username := requestUsername(r)
	max := s.upgradeMaxArtifactBytes()
	if err := r.ParseMultipartForm(max + (1 << 20)); err != nil {
		s.handleError(w, r, http.StatusRequestEntityTooLarge, "artifact exceeds size limit")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, "multipart field 'file' is required")
		return
	}
	defer file.Close()

	version := strings.TrimSpace(r.FormValue("version"))
	arch := strings.TrimSpace(r.FormValue("arch"))
	osName := strings.TrimSpace(r.FormValue("os"))
	minVersion := strings.TrimSpace(r.FormValue("min_version"))

	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to read artifact")
		return
	}
	shasum := hex.EncodeToString(h.Sum(nil))
	if max > 0 && n > max {
		s.handleError(w, r, http.StatusRequestEntityTooLarge,
			"artifact exceeds size limit")
		return
	}
	if err := validateArtifactMeta(version, minVersion, shasum, n, max); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// 同 version+arch+os 覆盖更新（幂等）：已有登记沿用原 id，否则发新 id
	id := protocol.GenerateID()
	now := time.Now()
	art := &storage.AgentArtifact{
		ID: id, Version: version, Arch: arch, OS: osName,
		Size: n, SHA256: shasum, MinVersion: minVersion,
		CreatedAt: now, UpdatedAt: now,
	}
	if existing, err := s.db.GetArtifact(version, arch, osName); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to load artifact")
		return
	} else if existing != nil {
		art.ID = existing.ID
		art.CreatedAt = existing.CreatedAt
	}
	if err := s.db.SaveArtifact(art); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to store artifact")
		return
	}
	s.audit.LogResource(username, audit.ActionAgentArtifact, audit.ResourceAgentArtifact,
		art.ID, map[string]interface{}{
			"version": version, "arch": arch, "os": osName,
			"size": n, "sha256": shasum, "minVersion": minVersion,
		}, s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusCreated, art)
}

// handleAgentUpgrade 单 agent 显式升级意图（POST /agents/{id}/upgrade，
// 见设计 §3.2）。非 bulk：只接受单 agent，无「全部」开关。
func (s *Server) handleAgentUpgrade(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Version              string   `json:"version"`
		Arch                 string   `json:"arch"`
		OS                   string   `json:"os"`
		RequiredCapabilities []string `json:"requiredCapabilities"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Version = strings.TrimSpace(body.Version)
	if body.Version == "" {
		s.handleError(w, r, http.StatusBadRequest, "version is required")
		return
	}

	agent, err := s.db.GetAgent(agentID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.handleError(w, r, http.StatusNotFound, "agent not found")
			return
		}
		s.handleError(w, r, http.StatusInternalServerError, "failed to load agent")
		return
	}
	if !s.agentOnline(agent) {
		s.handleError(w, r, http.StatusConflict, "agent is not online")
		return
	}
	// 只允许升级（拒绝降级与同级重升），版本比较用 semver-lite——
	// 先于产物解析：降级请求不因「恰好没登记该降级产物」而变 503
	if protocol.CompareVersion(body.Version, agent.Version) <= 0 {
		s.handleError(w, r, http.StatusConflict, fmt.Sprintf(
			"upgrade requires a higher version (agent %q, target %q)", agent.Version, body.Version))
		return
	}
	// 目标产物必须已登记（设计 §3.1）；arch/os 缺省 = 任意已登记平台
	art, err := s.resolveArtifact(body.Version, body.Arch, body.OS)
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, err.Error())
		return
	}
	// 产物声明的最低升级起点（§3.1 min_version 门控；空 = 不限）
	if !protocol.VersionSatisfies(agent.Version, art.MinVersion) {
		s.handleError(w, r, http.StatusConflict, fmt.Sprintf(
			"agent %q below artifact min_version %q", agent.Version, art.MinVersion))
		return
	}

	now := time.Now()
	intent := &storage.AgentUpgrade{
		ID:           protocol.GenerateID(),
		AgentID:      agentID,
		FromVersion:  agent.Version,
		ToVersion:    body.Version,
		ArtifactID:   art.ID,
		Status:       storage.UpgradeStatusPlanned,
		Operator:     requestUsername(r),
		RequiredCaps: body.RequiredCapabilities,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.db.CreateUpgrade(intent); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create upgrade intent")
		return
	}
	s.audit.LogResource(intent.Operator, audit.ActionAgentUpgrade, audit.ResourceAgentUpgrade,
		intent.ID, map[string]interface{}{
			"agentId": agentID, "fromVersion": intent.FromVersion,
			"toVersion": intent.ToVersion, "artifactId": art.ID,
			"status": intent.Status,
		}, s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusCreated, intent)
}

// resolveArtifact 解析目标产物：显式 arch/os 精确匹配，缺省匹配任意平台。
func (s *Server) resolveArtifact(version, arch, osName string) (*storage.AgentArtifact, error) {
	if arch != "" || osName != "" {
		art, err := s.db.GetArtifact(version, arch, osName)
		if err != nil {
			return nil, fmt.Errorf("failed to load artifact: %w", err)
		}
		if art == nil {
			return nil, fmt.Errorf("no artifact registered for version %s (arch %q, os %q)", version, arch, osName)
		}
		return art, nil
	}
	arts, err := s.db.ListArtifacts()
	if err != nil {
		return nil, fmt.Errorf("failed to load artifacts: %w", err)
	}
	for _, a := range arts {
		if a.Version == version {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no artifact registered for version %s", version)
}

// agentOnline agent 是否在线（DB 行 status 为准）。
func (s *Server) agentOnline(agent *storage.Agent) bool {
	return agent != nil && agent.Status == "online"
}

// requestUsername 取登录用户（无上下文时 "unknown"）。
func requestUsername(r *http.Request) string {
	if u, ok := auth.GetUserFromContext(r); ok && u.Username != "" {
		return u.Username
	}
	return "unknown"
}
