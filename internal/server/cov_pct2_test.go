package server

// cov_pct2_test.go 覆盖率补测（第二部分）：overlay cloud / recordings /
// server-backup / backups / audit / probe / stacks / docker / nas / smart /
// overlay / ddns 的错误分支。
// 手段：
//   - 直调 handler（httptest recorder），构造畸形路径/方法/请求体
//   - closed db 覆盖「首个 DB 操作即失败」；covPctQueryOnlyDB（见
//     cov_pct_test.go）覆盖「读成功→写失败」
//   - closed agent（Close 不 Unregister）让 CallAgent 立即返回
//     "agent X is closed"：agent ID 精心取名为含 "busy"/"not found" 以命中
//     handleStackRPC 的错误码映射分支
//   - NaN payload 使 protocol.DecodePayload 的 json.Marshal 失败，覆盖
//     DecodeRPCResponse 错误分支
//   - 失败 writer（Write 恒报错）覆盖 CSV 导出的 writer.Write 分支
//   - registry 内反复注册/注销同一 agent 的 toggle goroutine 打开两次
//     registry.Get 之间的竞态窗口（轮询 + deadline，禁止固定睡眠）

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ overlay cloud（api_overlay_cloud.go）============

// TestCovPctOverlayCloudListBadMethod 覆盖列表端点非 GET 的 405 分支
// （api_overlay_cloud.go L49-52）。
func TestCovPctOverlayCloudListBadMethod(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil, nil)
	w := doOverlayCloud(s, http.MethodPost, "/api/overlay/cloud", "")
	covWantCode(t, "list 405", w, http.StatusMethodNotAllowed)
}

// TestCovPctOverlayCloudListTailscaleError 覆盖列表中 Tailscale 段
// Devices 失败降级分支（api_overlay_cloud.go L97-100）：云端 500。
func TestCovPctOverlayCloudListTailscaleError(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil,
		func(r *http.Request) (int, string) { return 500, `{"error":"ts down"}` })
	w := doOverlayCloud(s, http.MethodGet, "/api/overlay/cloud", "")
	if w.Code != http.StatusOK {
		t.Fatalf("degraded list = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "overlay cloud api status 500") {
		t.Errorf("ts segment should carry error: %s", w.Body.String())
	}
}

// TestCovPctOverlayCloudZTBadSubpath 覆盖 ZT member 子路径形态不完整的
// 404 分支（api_overlay_cloud.go L116-119）：缺 member 段。
func TestCovPctOverlayCloudZTBadSubpath(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil, nil)
	w := doOverlayCloud(s, http.MethodPost, "/api/overlay/cloud/zerotier/networks/xyz", `{}`)
	covWantCode(t, "zt bad subpath 404", w, http.StatusNotFound)
}

// TestCovPctOverlayCloudZTAuthzUpstreamFail 覆盖 ZT 授权变更的云端错误
// 分支（api_overlay_cloud.go L143-146）：SetMemberAuthorized 500 → 502。
func TestCovPctOverlayCloudZTAuthzUpstreamFail(t *testing.T) {
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { return 500, `{"error":"down"}` }, nil)
	w := doOverlayCloud(s, http.MethodPost,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12",
		`{"authorized":true}`)
	covWantCode(t, "zt authz 502", w, http.StatusBadGateway)
}

// TestCovPctOverlayCloudZTBadMethod 覆盖 ZT member 变更的非法方法分支
// （api_overlay_cloud.go L160-161）：PATCH → 405。
func TestCovPctOverlayCloudZTBadMethod(t *testing.T) {
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { return 200, `{}` }, nil)
	w := doOverlayCloud(s, http.MethodPatch,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", "")
	covWantCode(t, "zt 405", w, http.StatusMethodNotAllowed)
}

// TestCovPctOverlayCloudTSBadSubpath 覆盖 TS device 子路径空的 404 分支
// （api_overlay_cloud.go L168-171）。
func TestCovPctOverlayCloudTSBadSubpath(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil, nil)
	w := doOverlayCloud(s, http.MethodDelete, "/api/overlay/cloud/tailscale/devices/", "")
	covWantCode(t, "ts bad subpath 404", w, http.StatusNotFound)
}

// TestCovPctOverlayCloudTSAuthzUpstreamFail 覆盖 TS 授权的云端错误分支
// （api_overlay_cloud.go L189-192）：AuthorizeDevice 500 → 502。
func TestCovPctOverlayCloudTSAuthzUpstreamFail(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil,
		func(r *http.Request) (int, string) { return 500, `{"error":"down"}` })
	w := doOverlayCloud(s, http.MethodPost, "/api/overlay/cloud/tailscale/devices/123/authorize", "")
	covWantCode(t, "ts authz 502", w, http.StatusBadGateway)
}

// TestCovPctOverlayCloudTSDeleteUpstreamFail 覆盖 TS 除名的云端错误分支
// （api_overlay_cloud.go L197-200）：DeleteDevice 500 → 502。
func TestCovPctOverlayCloudTSDeleteUpstreamFail(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil,
		func(r *http.Request) (int, string) { return 500, `{"error":"down"}` })
	w := doOverlayCloud(s, http.MethodDelete, "/api/overlay/cloud/tailscale/devices/123", "")
	covWantCode(t, "ts delete 502", w, http.StatusBadGateway)
}

// TestCovPctOverlayCloudTSBadMethod 覆盖 TS device 的非法方法分支
// （api_overlay_cloud.go L204-205）：PUT → 405。
func TestCovPctOverlayCloudTSBadMethod(t *testing.T) {
	s := newOverlayCloudTestServer(t, nil,
		func(r *http.Request) (int, string) { return 200, `{}` })
	w := doOverlayCloud(s, http.MethodPut, "/api/overlay/cloud/tailscale/devices/123", "")
	covWantCode(t, "ts 405", w, http.StatusMethodNotAllowed)
}

// TestCovPctOverlayCloudNilRegistryIdentity 覆盖 overlayIdentitySets 的
// registry 为 nil 分支（api_overlay_cloud.go L215-217）：直构空 Server。
func TestCovPctOverlayCloudNilRegistryIdentity(t *testing.T) {
	s := &Server{}
	zt, ts := s.overlayIdentitySets()
	if len(zt) != 0 || len(ts) != 0 {
		t.Fatalf("nil registry identity sets = %v %v, want empty", zt, ts)
	}
}

// TestCovPctOverlayCloudAuditWithUser 覆盖 auditOverlayCloud 携带用户上下文
// 分支（api_overlay_cloud.go L278-280）：经 auth.Middleware 注入 admin，
// 审计记录 username 非空。
func TestCovPctOverlayCloudAuditWithUser(t *testing.T) {
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { return 200, `{}` }, nil)
	r := covReq(http.MethodPost,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12",
		strings.NewReader(`{"authorized":true}`))
	token, err := auth.GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	w := callWithAuth(s, s.handleOverlayCloud, r)
	covWantCode(t, "authz ok", w, http.StatusOK)

	logs, _, err := s.db.GetAuditLogs(0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.Action == "overlay_authz" && l.Username == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit with username=admin not found: %+v", logs)
	}
}

// ============ recordings（api_recordings.go）============

// TestCovPctRecordingSyncRemoteBadMethod 覆盖 sync-remote 子路径非 POST 的
// 405 分支（api_recordings.go L56-59）。
func TestCovPctRecordingSyncRemoteBadMethod(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleRecordings(rec, covReq(http.MethodGet, "/recordings/abc/sync-remote", nil))
	covWantCode(t, "sync-remote 405", rec, http.StatusMethodNotAllowed)
}

// TestCovPctRecordingListNilToEmpty 覆盖录制列表空表 nil → 空数组分支
// （api_recordings.go L81-83）：空库 GORM Find 返回 nil slice。
func TestCovPctRecordingListNilToEmpty(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleRecordingsList(rec, covReq(http.MethodGet, "/recordings", nil))
	covWantCode(t, "list ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("empty list should marshal as [], got %s", rec.Body.String())
	}
}

// TestCovPctRecordingConfigBadJSON 覆盖录制配置写入的坏 JSON 分支
// （api_recordings.go L146-149）。
func TestCovPctRecordingConfigBadJSON(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleRecordingsConfig(rec, covReq(http.MethodPut, "/recordings/config", strings.NewReader(`{`)))
	covWantCode(t, "config bad json", rec, http.StatusBadRequest)
}

// TestCovPctRecordingConfigEnabledTrue 覆盖配置写入 enabled=true 的取值
// 分支（api_recordings.go L152-154）。
func TestCovPctRecordingConfigEnabledTrue(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleRecordingsConfig(rec, covReq(http.MethodPut, "/recordings/config",
		strings.NewReader(`{"enabled":true}`)))
	covWantCode(t, "config enabled ok", rec, http.StatusOK)
	v, err := s.db.GetSetting(RecordingEnabledSettingKey)
	if err != nil || v != "true" {
		t.Fatalf("enabled setting = %q %v, want true", v, err)
	}
}

// TestCovPctRecordingConfigSetFails 覆盖录制配置三个 Setting 写入失败分支
// （api_recordings.go L155-158 / L166-169 / L179-182）：只读库写被拒。
func TestCovPctRecordingConfigSetFails(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	cases := []struct {
		name, body string
		want       int
	}{
		{"enabled set fail", `{"enabled":true}`, http.StatusInternalServerError},
		{"retention set fail", `{"retention_days":30}`, http.StatusInternalServerError},
		{"remote dest set fail", `{"remote_dest":"r:bak"}`, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		rec := covRec()
		s.handleRecordingsConfig(rec, covReq(http.MethodPut, "/recordings/config", strings.NewReader(tc.body)))
		covWantCode(t, tc.name, rec, tc.want)
	}
}

// TestCovPctRecordingConfigBadMethod 覆盖录制配置的非法方法分支
// （api_recordings.go L190-191）：PATCH → 405。
func TestCovPctRecordingConfigBadMethod(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleRecordingsConfig(rec, covReq(http.MethodPatch, "/recordings/config", nil))
	covWantCode(t, "config 405", rec, http.StatusMethodNotAllowed)
}

// TestCovPctRecordingSyncRemoteFileMissing 覆盖补推时本地文件缺失分支
// （api_recordings.go L210-213）：元数据在库、.cast 文件不存在 → 404。
func TestCovPctRecordingSyncRemoteFileMissing(t *testing.T) {
	s := covNewServer(t)
	if err := s.db.SetSetting(RecordingRemoteDestSettingKey, "r:bak"); err != nil {
		t.Fatal(err)
	}
	sid := "12345678-1234-1234-1234-123456789abc"
	if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{SessionID: sid, AgentID: "a1"}); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleRecordingSyncRemote(rec, covReq(http.MethodPost, "/recordings/"+sid+"/sync-remote", nil), sid)
	covWantCode(t, "file missing 404", rec, http.StatusNotFound)
}

// ============ server backups（api_server_backup.go）============

// TestCovPctServerBackupSyncRemoteBadMethod 覆盖补推端点非 POST 的 405
// 分支（api_server_backup.go L52-55）：文件名合法、方法错误。
func TestCovPctServerBackupSyncRemoteBadMethod(t *testing.T) {
	s := newBackupTestServer(t)
	rec := covRec()
	s.handleServerBackups(rec, covReq(http.MethodGet,
		"/server-backups/cockpit-20200101-000000.db/sync-remote", nil))
	covWantCode(t, "sync-remote 405", rec, http.StatusMethodNotAllowed)
}

// TestCovPctServerBackupRemoteDestSetFail 覆盖配置写入 remote_dest 的
// SetSetting 失败分支（api_server_backup.go L139-142）：只读库。
func TestCovPctServerBackupRemoteDestSetFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, nil)
	rec := covRec()
	s.handleServerBackupsConfig(rec, covReq(http.MethodPut, "/api/server-backups/config",
		strings.NewReader(`{"remote_dest":"r:bak"}`)))
	covWantCode(t, "remote dest set 500", rec, http.StatusInternalServerError)
}

// ============ backups（api_backups.go）============

// TestCovPctBackupConfigsBadMethod 覆盖 config 单条端点的非法方法分支
// （api_backups.go L73-74）：PATCH → 405。
func TestCovPctBackupConfigsBadMethod(t *testing.T) {
	s := newBackupTestServer(t)
	rec := covRec()
	s.handleBackupsAPI(rec, covReq(http.MethodPatch, "/api/backups/configs/1", nil))
	covWantCode(t, "configs 405", rec, http.StatusMethodNotAllowed)
}

// TestCovPctBackupCreateRcloneMissing 覆盖创建配置时异地启用但 agent 无
// rclone 能力分支（api_backups.go L183-185）：capability 存在但 metadata
// 无 rclone 键 → 400。
func TestCovPctBackupCreateRcloneMissing(t *testing.T) {
	s := newBackupTestServer(t)
	covFakeAgent(t, s, "bak-norclone", []string{"backup"}, nil)
	rec := covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs",
		strings.NewReader(`{"agent_id":"bak-norclone","name":"bak","sources":["/etc"],"dest_dir":"/b","schedule":"manual","remote_dest":"r:bak"}`)))
	covWantCode(t, "create rclone missing 400", rec, http.StatusBadRequest)
}

// TestCovPctBackupUpdateRcloneMissing 覆盖更新配置时的同款分支
// （api_backups.go L297-299）。
func TestCovPctBackupUpdateRcloneMissing(t *testing.T) {
	s := newBackupTestServer(t)
	covFakeAgent(t, s, "bak-norclone", []string{"backup"}, nil)
	cfg := &storage.BackupConfig{AgentID: "bak-norclone", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"bak-norclone","name":"bak","sources":["/etc"],"dest_dir":"/b","schedule":"manual","remote_dest":"r:bak"}`)), 1)
	covWantCode(t, "update rclone missing 400", rec, http.StatusBadRequest)
}

// TestCovPctBackupSyncAgentUnreachable 覆盖文件补推的 CallAgent 失败分支
// （api_backups.go L453-456）：agent 已 Close（registry 仍命中）→ 502。
func TestCovPctBackupSyncAgentUnreachable(t *testing.T) {
	s := newBackupTestServer(t)
	closed := NewAgent("bak-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	cfg := &storage.BackupConfig{AgentID: "bak-closed", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual", RemoteDest: "r:bak"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupFileSyncRemote(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/sync-remote",
		strings.NewReader(`{"name":"20240101-000000.node1.tar.gz"}`)), cfg)
	covWantCode(t, "sync 502", rec, http.StatusBadGateway)
}

// TestCovPctBackupSyncBadPayload 覆盖文件补推的 DecodeRPCResponse 失败
// 分支（api_backups.go L458-461）：payload 含 NaN 使 json.Marshal 失败。
func TestCovPctBackupSyncBadPayload(t *testing.T) {
	s := newBackupTestServer(t)
	covFakeAgent(t, s, "bak-nan", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"x": math.NaN()})
	})
	cfg := &storage.BackupConfig{AgentID: "bak-nan", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual", RemoteDest: "r:bak"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupFileSyncRemote(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/sync-remote",
		strings.NewReader(`{"name":"20240101-000000.node1.tar.gz"}`)), cfg)
	covWantCode(t, "sync bad payload 502", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "failed to sync file to remote") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// TestCovPctBackupSyncAgentErrorEmpty 覆盖文件补推的 agent 错误但错误消息
// 为空分支（api_backups.go L465-467）：默认文案兜底。
func TestCovPctBackupSyncAgentErrorEmpty(t *testing.T) {
	s := newBackupTestServer(t)
	covFakeAgent(t, s, "bak-err-empty", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("")
	})
	cfg := &storage.BackupConfig{AgentID: "bak-err-empty", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual", RemoteDest: "r:bak"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupFileSyncRemote(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/sync-remote",
		strings.NewReader(`{"name":"20240101-000000.node1.tar.gz"}`)), cfg)
	covWantCode(t, "sync agent error 502", rec, http.StatusBadGateway)
	if !strings.Contains(rec.Body.String(), "failed to sync file to remote") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// ============ audit（api_audit.go）============

// TestCovPctAuditStatsOK 覆盖审计统计正常路径（GetAuditLogStats 对底层
// Count 错误一律吞掉恒返回 nil error，handler 的 500 分支无触发途径）。
func TestCovPctAuditStatsOK(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleAuditLogStats(rec, covReq(http.MethodGet, "/api/audit/stats", nil))
	covWantCode(t, "stats ok", rec, http.StatusOK)
}

// covPctErrWriter Write 恒失败的 ResponseWriter（触发 CSV writer.Write 错误分支）
type covPctErrWriter struct{}

func (covPctErrWriter) Header() http.Header       { return http.Header{} }
func (covPctErrWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func (covPctErrWriter) WriteHeader(int)           {}

// TestCovPctAuditExportWriterFail 覆盖 CSV 导出的表头写出失败分支
// （api_audit.go L105-108）：底层 Write 恒报错。
func TestCovPctAuditExportWriterFail(t *testing.T) {
	s := covNewServer(t)
	s.handleAuditLogsExport(covPctErrWriter{}, covReq(http.MethodGet, "/api/audit/export", nil))
}

// ============ probe（api_probe.go）============

// TestCovPctProbeHistoryEmpty 覆盖探测历史空表 nil → 空数组分支
// （api_probe.go L229-231）。
func TestCovPctProbeHistoryEmpty(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleProbeHistory(rec, covReq(http.MethodGet,
		"/api/probe/history?resource_type=service&resource_id=svc-1", nil))
	covWantCode(t, "probe history ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"results":[]`) {
		t.Errorf("empty results should marshal as [], got %s", rec.Body.String())
	}
}

// ============ stacks / docker 的 agent 错误映射（api_stacks.go / api_docker.go）============

// TestCovPctStackRPCClosedAgentBusy 覆盖 handleStackRPC 的 busy → 409 映射
// （api_stacks.go L192-194）：closed agent 的报错含其 ID，取名含 "busy"。
func TestCovPctStackRPCClosedAgentBusy(t *testing.T) {
	s := covNewServer(t)
	closed := NewAgent("busy-agent", nil)
	closed.Capabilities = []protocol.Capability{{Type: "docker"}}
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec := covRec()
	s.handleStackRPC(rec, covReq(http.MethodGet, "/api/stacks/agents/busy-agent", nil),
		"busy-agent", "stack.list", map[string]interface{}{}, "")
	covWantCode(t, "busy 409", rec, http.StatusConflict)
}

// TestCovPctStackRPCClosedAgentNotFound 覆盖 handleStackRPC 的
// "not found" → 404 映射（api_stacks.go L194-196）：ID 含 "not found"
// 但不含 "busy"。
func TestCovPctStackRPCClosedAgentNotFound(t *testing.T) {
	s := covNewServer(t)
	closed := NewAgent("no not found", nil)
	closed.Capabilities = []protocol.Capability{{Type: "docker"}}
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec := covRec()
	s.handleStackRPC(rec, covReq(http.MethodGet, "/api/stacks/agents/no%20not%20found", nil),
		"no not found", "stack.list", map[string]interface{}{}, "")
	covWantCode(t, "not found 404", rec, http.StatusNotFound)
}

// TestCovPctStackHistoryEmpty 覆盖部署历史空表 nil → 空数组分支
// （api_stacks.go L343-345）。
func TestCovPctStackHistoryEmpty(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleStackHistory(rec, covReq(http.MethodGet, "/api/stacks/agents/a1/names/web/history", nil), "a1", "web")
	covWantCode(t, "history ok", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"deployments":[]`) {
		t.Errorf("empty deployments should marshal as [], got %s", rec.Body.String())
	}
}

// covPctRaceAgent 起一个 toggle goroutine 周期性注册/注销同一个带 docker
// 能力的 agent，打开 handler 内两次 registry.Get 之间的竞态窗口。agent 的
// Send 由常驻 consumer 即时回 error 响应，保证未命中窗口的调用快速返回
// （不阻塞 30s）。返回停止函数（close 即停）。
func covPctRaceAgent(t *testing.T, s *Server, id string) chan struct{} {
	t.Helper()
	stop := make(chan struct{})
	agent := NewAgent(id, nil)
	agent.Capabilities = []protocol.Capability{{Type: "docker"}}
	go func() {
		for msg := range agent.Send {
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, covErrPayload("toggled away"))
			resp.ID = msg.ID
			s.handleRPCResponse(resp)
		}
	}()
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := s.registry.Register(agent); err == nil {
				s.registry.Unregister(id)
			}
		}
	}()
	return stop
}

// covPctRaceUntil 轮询调用 probe 直到命中 ErrAgentNotFound 分支（404 且
// body 含错误码 agent_not_found；能力前置检查失败是纯文本，可区分）。
// 轮询 + deadline，无固定睡眠。
func covPctRaceUntil(t *testing.T, probe func() (int, string)) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		code, body := probe()
		if code == http.StatusNotFound && strings.Contains(body, "agent_not_found") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("race window not hit within deadline, last: %d %s", code, body)
		}
	}
}

// TestCovPctStackRPCRaceUnregister 覆盖 handleStackRPC 的 ErrAgentNotFound
// 分支（api_stacks.go L190-192）：requireStackAgent 命中后、CallAgent 前
// agent 被并发注销。
func TestCovPctStackRPCRaceUnregister(t *testing.T) {
	s := covNewServer(t)
	stop := covPctRaceAgent(t, s, "race-stack")
	defer close(stop)
	covPctRaceUntil(t, func() (int, string) {
		rec := covRec()
		s.handleStackRPC(rec, covReq(http.MethodGet, "/api/stacks/agents/race-stack", nil),
			"race-stack", "stack.list", map[string]interface{}{}, "")
		return rec.Code, rec.Body.String()
	})
}

// TestCovPctAgentStacksRaceUnregister 覆盖 handleAgentStacks 的
// ErrAgentNotFound 分支（api_stacks.go L403-405）。
func TestCovPctAgentStacksRaceUnregister(t *testing.T) {
	s := covNewServer(t)
	stop := covPctRaceAgent(t, s, "race-stacklist")
	defer close(stop)
	covPctRaceUntil(t, func() (int, string) {
		rec := covRec()
		s.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/race-stacklist", nil), "race-stacklist")
		return rec.Code, rec.Body.String()
	})
}

// TestCovPctDockerRaceUnregister 覆盖 handleDocker 的 ErrAgentNotFound
// 分支（api_docker.go L43-45）。
func TestCovPctDockerRaceUnregister(t *testing.T) {
	s := covNewServer(t)
	stop := covPctRaceAgent(t, s, "race-docker")
	defer close(stop)
	covPctRaceUntil(t, func() (int, string) {
		rec := covRec()
		s.handleDocker(rec, covReq(http.MethodGet, "/api/docker/agents/race-docker/containers", nil))
		return rec.Code, rec.Body.String()
	})
}

// ============ nas / smart / overlay 的 agent 前置与转发（api_nas.go / api_smart.go / api_overlay.go）============

// TestCovPctNASAgentOffline 覆盖 NAS 代理的 agent 不在册分支
// （api_nas.go L65-68）：registry.Get miss → 503。
func TestCovPctNASAgentOffline(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/nas/status", nil), "ghost/nas/status")
	covWantCode(t, "nas offline 503", rec, http.StatusServiceUnavailable)
}

// TestCovPctNASClosedAgent 覆盖 NAS 代理的 CallAgent 失败分支
// （api_nas.go L86-89）：closed agent → 502。
func TestCovPctNASClosedAgent(t *testing.T) {
	s := covNewServer(t)
	closed := NewAgent("nas-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec := covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/nas-closed/nas/status", nil), "nas-closed/nas/status")
	covWantCode(t, "nas 502", rec, http.StatusBadGateway)
}

// TestCovPctSmartClosedAgent 覆盖 SMART 代理的 CallAgent 失败分支
// （api_smart.go L76-79）。
func TestCovPctSmartClosedAgent(t *testing.T) {
	s := covNewServer(t)
	closed := NewAgent("smart-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec := covRec()
	s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/smart-closed/smart/status", nil), "smart-closed/smart/status")
	covWantCode(t, "smart 502", rec, http.StatusBadGateway)
}

// TestCovPctOverlayAgentClosed 覆盖 overlay 代理的 CallAgent 失败分支
// （api_overlay.go L44-47）。
func TestCovPctOverlayAgentClosed(t *testing.T) {
	s := covNewServer(t)
	closed := NewAgent("ov-closed", nil)
	if err := s.registry.Register(closed); err != nil {
		t.Fatal(err)
	}
	closed.Close()
	rec := covRec()
	s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/ov-closed/overlay/status", nil), "ov-closed/overlay/status")
	covWantCode(t, "overlay 502", rec, http.StatusBadGateway)
}

// ============ ddns（api_ddns.go）============

// TestCovPctDDNSUpdateDBFail 覆盖 DDNS 更新的 UpdateDDNSConfig 失败分支
// （api_ddns.go L159-162）：只读库读成功、校验通过、写被拒。
func TestCovPctDDNSUpdateDBFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, func(db *storage.DB) {
		if err := db.CreateDDNSConfig(&storage.DDNSConfig{
			AgentID: "a1", ZoneID: "z1", ZoneName: "example.com",
			RecordName: "home.example.com", Type: "A",
		}); err != nil {
			t.Fatalf("seed ddns: %v", err)
		}
	})
	body := `{"agentId":"a1","zoneId":"z1","zoneName":"example.com","recordName":"home.example.com","type":"A","enabled":true}`
	rec := covRec()
	s.handleDDNSUpdate(rec, covReq(http.MethodPut, "/api/ddns/configs/1", strings.NewReader(body)), 1)
	covWantCode(t, "ddns update 500", rec, http.StatusInternalServerError)
}

// TestCovPctDDNSDeleteDBFail 覆盖 DDNS 删除的 DeleteDDNSConfig 失败分支
// （api_ddns.go L173-176）：只读库读成功、删除被拒。
func TestCovPctDDNSDeleteDBFail(t *testing.T) {
	s := covPctQueryOnlyDB(t, func(db *storage.DB) {
		if err := db.CreateDDNSConfig(&storage.DDNSConfig{
			AgentID: "a1", ZoneID: "z1", ZoneName: "example.com",
			RecordName: "home.example.com", Type: "A",
		}); err != nil {
			t.Fatalf("seed ddns: %v", err)
		}
	})
	rec := covRec()
	s.handleDDNSDelete(rec, covReq(http.MethodDelete, "/api/ddns/configs/1", nil), 1)
	covWantCode(t, "ddns delete 500", rec, http.StatusInternalServerError)
}
