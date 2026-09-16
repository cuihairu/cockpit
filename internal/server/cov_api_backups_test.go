package server

// api_backups.go 的覆盖率补充测试：路由分发、withBackupConfig、各 handler 错误分支。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covStuckAgent 注册一个不读 Send 的 agent 并填满其缓冲（256），
// 使后续每个 CallAgent 都在 5s 后触发 "send timeout"。
func covStuckAgent(t *testing.T, s *Server, agentID string) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	for i := 0; i < 256; i++ {
		agent.Send <- protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{})
	}
	t.Cleanup(func() { s.registry.Unregister(agentID) })
}

// covBadPayloadAgent 应答 status 非字符串的 payload，触发 DecodeRPCResponse 错误
func covBadPayloadAgent(t *testing.T, s *Server, agentID string) {
	covFakeAgent(t, s, agentID, nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": 123}
	})
}

// covSeedBackupCfg 落库一个备份配置并返回加载后的完整对象
func covSeedBackupCfg(t *testing.T, s *Server, agentID, name string) *storage.BackupConfig {
	t.Helper()
	cfg := newBackupCfg(agentID, name, "manual", true)
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.db.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// ============ registerBackupsAPI ============

func TestCovRegisterBackupsAPI(t *testing.T) {
	s := newBackupTestServer(t)
	mux := http.NewServeMux()
	s.registerBackupsAPI(mux)

	// 无认证 → 401（auth 中间件拦截）
	rec := covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/backups/configs", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	// 带认证 → 正常分发（token 由该 server 的 authService 签发）
	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/backups/configs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "with auth", rec, http.StatusOK)
}

// ============ handleBackupsAPI 路由分发 ============

func TestCovBackupsAPIDispatch(t *testing.T) {
	s := newBackupTestServer(t)
	// cfg 1：agent 离线（run → 503，不产生后台任务）
	if err := s.db.CreateBackupConfig(newBackupCfg("ghost-agent", "off", "manual", true)); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"configs list", http.MethodGet, "/api/backups/configs", "", http.StatusOK},
		{"configs create", http.MethodPost, "/api/backups/configs", validBackupReq(), http.StatusBadRequest}, // agent a1 不在线 → 400
		{"runs list", http.MethodGet, "/api/backups/runs", "", http.StatusOK},
		{"runs list limit", http.MethodGet, "/api/backups/runs?limit=10", "", http.StatusOK},
		{"runs list bad limit", http.MethodGet, "/api/backups/runs?limit=abc", "", http.StatusOK},
		{"config id 405", http.MethodPatch, "/api/backups/configs/1", "", http.StatusMethodNotAllowed},
		{"config bad id", http.MethodDelete, "/api/backups/configs/abc", "", http.StatusBadRequest},
		{"config update missing", http.MethodPut, "/api/backups/configs/999", validBackupReq(), http.StatusNotFound},
		{"config delete missing", http.MethodDelete, "/api/backups/configs/999", "", http.StatusNotFound},
		{"run bad id", http.MethodPost, "/api/backups/configs/abc/run", "", http.StatusBadRequest},
		{"run missing cfg", http.MethodPost, "/api/backups/configs/999/run", "", http.StatusNotFound},
		{"runs missing cfg", http.MethodGet, "/api/backups/configs/999/runs", "", http.StatusNotFound},
		{"files missing cfg", http.MethodGet, "/api/backups/configs/999/files", "", http.StatusNotFound},
		{"file delete missing cfg", http.MethodPost, "/api/backups/configs/999/files/delete", `{"name":"a.tar.gz"}`, http.StatusNotFound},
		{"download missing cfg", http.MethodGet, "/api/backups/configs/999/files/download?name=a.tar.gz", "", http.StatusNotFound},
		{"task missing cfg", http.MethodGet, "/api/backups/configs/999/tasks/t1", "", http.StatusNotFound},
		{"restore missing cfg", http.MethodPost, "/api/backups/configs/999/restore", `{"file":"a.tar.gz","dest_dir":"/r","confirm_name":"a.tar.gz"}`, http.StatusNotFound},
		{"run offline agent", http.MethodPost, "/api/backups/configs/1/run", "", http.StatusServiceUnavailable},
		{"unknown path", http.MethodGet, "/api/backups/whatever", "", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body io.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			}
			rec := covRec()
			s.handleBackupsAPI(rec, covReq(c.method, c.path, body))
			covWantCode(t, c.name, rec, c.want)
		})
	}
}

// ============ 配置 handler 补充分支 ============

func TestCovBackupConfigHandlerErrors(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	// 创建：非法 JSON → 400
	rec := covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs", strings.NewReader("not-json")))
	covWantCode(t, "create bad json", rec, http.StatusBadRequest)

	// 创建：缺 agent_id → 400
	rec = covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs",
		strings.NewReader(`{"name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual"}`)))
	covWantCode(t, "create no agent", rec, http.StatusBadRequest)

	// 创建：显式 enabled=false 分支
	rec = covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs",
		strings.NewReader(`{"agent_id":"a1","name":"etc2","sources":["/a"],"dest_dir":"/b","schedule":"manual","enabled":false}`)))
	covWantCode(t, "create disabled", rec, http.StatusCreated)

	// 更新：目标不存在 → 404
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/999", strings.NewReader(validBackupReq())), 999)
	covWantCode(t, "update missing", rec, http.StatusNotFound)

	// 更新：非法 JSON → 400
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/1", strings.NewReader("not-json")), cfg.ID)
	covWantCode(t, "update bad json", rec, http.StatusBadRequest)

	// 更新：校验失败（缺 agent_id）→ 400
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual"}`)), cfg.ID)
	covWantCode(t, "update invalid", rec, http.StatusBadRequest)

	// 更新：agent 不在线 → 400
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"ghost","name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual"}`)), cfg.ID)
	covWantCode(t, "update agent offline", rec, http.StatusBadRequest)

	// 更新：retention 越界 → 400
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/1",
		strings.NewReader(`{"agent_id":"a1","name":"ok","sources":["/a"],"dest_dir":"/b","schedule":"manual","retention":999}`)), cfg.ID)
	covWantCode(t, "update retention over", rec, http.StatusBadRequest)

	// 更新：运行中 → 409
	busy := newBackupCfg("a1", "busy", "manual", true)
	busy.LastStatus = "running"
	if err := s.db.CreateBackupConfig(busy); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleBackupConfigUpdate(rec, covReq(http.MethodPut, "/api/backups/configs/2", strings.NewReader(validBackupReq())), busy.ID)
	covWantCode(t, "update while running", rec, http.StatusConflict)

	// 删除：目标不存在 → 404
	rec = covRec()
	s.handleBackupConfigDelete(rec, covReq(http.MethodDelete, "/api/backups/configs/999", nil), 999)
	covWantCode(t, "delete missing", rec, http.StatusNotFound)
}

// closed db 触发的 500 分支
func TestCovBackupHandlerDBError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	covCloseDB(t, s)

	// 配置列表 → 500
	rec := covRec()
	s.handleBackupConfigsList(rec, covReq(http.MethodGet, "/api/backups/configs", nil))
	covWantCode(t, "configs list", rec, http.StatusInternalServerError)

	// 创建配置 → 500
	rec = covRec()
	s.handleBackupConfigCreate(rec, covReq(http.MethodPost, "/api/backups/configs", strings.NewReader(validBackupReq())))
	covWantCode(t, "config create", rec, http.StatusInternalServerError)

	// 运行历史 → 500
	rec = covRec()
	s.handleBackupRunsList(rec, covReq(http.MethodGet, "/api/backups/runs", nil))
	covWantCode(t, "runs list", rec, http.StatusInternalServerError)
}

// startBackupRun 失败 → 500（配置名非法，无需 agent 往返）
func TestCovBackupRunStartError(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	cfg := covSeedBackupCfg(t, s, "a1", "Bad Name!")

	rec := covRec()
	s.handleBackupRun(rec, covReq(http.MethodPost, "/api/backups/configs/1/run", nil), cfg)
	covWantCode(t, "run start error", rec, http.StatusInternalServerError)
}

// ============ 文件浏览 / 删除 / 恢复 / 任务 / 下载 ============

func TestCovBackupFilesBranches(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method == "backup.list" {
			return map[string]interface{}{"files": []interface{}{"a.tar.gz"}}, ""
		}
		return nil, "unexpected method " + method
	})
	cfg := covSeedBackupCfg(t, s, "a1", "etc")
	ghostCfg := newBackupCfg("ghost-agent", "ghost", "manual", true)
	if err := s.db.CreateBackupConfig(ghostCfg); err != nil {
		t.Fatal(err)
	}

	// 成功
	rec := covRec()
	s.handleBackupFiles(rec, covReq(http.MethodGet, "/api/backups/configs/1/files", nil), cfg)
	covWantCode(t, "files ok", rec, http.StatusOK)

	// agent 离线 → 503
	rec = covRec()
	s.handleBackupFiles(rec, covReq(http.MethodGet, "/api/backups/configs/2/files", nil), ghostCfg)
	covWantCode(t, "files offline", rec, http.StatusServiceUnavailable)

	// agent 返回 rpc error → 502
	s2 := newBackupTestServer(t)
	withFakeBackupAgent(t, s2, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "boom"
	})
	cfg2 := covSeedBackupCfg(t, s2, "a1", "etc")
	rec = covRec()
	s2.handleBackupFiles(rec, covReq(http.MethodGet, "/api/backups/configs/1/files", nil), cfg2)
	covWantCode(t, "files rpc error", rec, http.StatusBadGateway)
}

// 应答 payload 非法（status 非字符串）→ DecodeRPCResponse 错误 → 502
func TestCovBackupFilesDecodeError(t *testing.T) {
	s := newBackupTestServer(t)
	covBadPayloadAgent(t, s, "a1")
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	rec := covRec()
	s.handleBackupFiles(rec, covReq(http.MethodGet, "/api/backups/configs/1/files", nil), cfg)
	covWantCode(t, "files decode error", rec, http.StatusBadGateway)
}

func TestCovBackupFileDeleteBranches(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	// 非法 JSON / 空 name → 400
	rec := covRec()
	s.handleBackupFileDelete(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/delete", strings.NewReader("not-json")), cfg)
	covWantCode(t, "bad json", rec, http.StatusBadRequest)

	rec = covRec()
	s.handleBackupFileDelete(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/delete", strings.NewReader(`{}`)), cfg)
	covWantCode(t, "empty name", rec, http.StatusBadRequest)

	// agent 离线 → 503
	ghostCfg := newBackupCfg("ghost-agent", "ghost", "manual", true)
	if err := s.db.CreateBackupConfig(ghostCfg); err != nil {
		t.Fatal(err)
	}
	rec = covRec()
	s.handleBackupFileDelete(rec, covReq(http.MethodPost, "/api/backups/configs/2/files/delete",
		strings.NewReader(`{"name":"etc-20260914-030000.tar.gz"}`)), ghostCfg)
	covWantCode(t, "offline", rec, http.StatusServiceUnavailable)

	// 成功（带认证上下文 → 审计 username 分支）
	req := covAuthReq(http.MethodPost, "/api/backups/configs/1/files/delete",
		strings.NewReader(`{"name":"etc-20260914-030000.tar.gz"}`), "1", "admin", "admin")
	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleBackupFileDelete(w, r, cfg)
	}, req)
	covWantCode(t, "deleted", rec, http.StatusOK)

	// agent rpc error → 502
	s2 := newBackupTestServer(t)
	withFakeBackupAgent(t, s2, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "boom"
	})
	cfg2 := covSeedBackupCfg(t, s2, "a1", "etc")
	rec = covRec()
	s2.handleBackupFileDelete(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/delete",
		strings.NewReader(`{"name":"etc-20260914-030000.tar.gz"}`)), cfg2)
	covWantCode(t, "rpc error", rec, http.StatusBadGateway)
}

func TestCovBackupRestoreAndTaskExtraBranches(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	// restore：非法 JSON → 400
	rec := covRec()
	s.handleBackupRestore(rec, covReq(http.MethodPost, "/r", strings.NewReader("not-json")), cfg)
	covWantCode(t, "restore bad json", rec, http.StatusBadRequest)

	// restore：agent 离线 → 503
	ghostCfg := newBackupCfg("ghost-agent", "ghost", "manual", true)
	if err := s.db.CreateBackupConfig(ghostCfg); err != nil {
		t.Fatal(err)
	}
	body := `{"file":"etc-20260914-030000.tar.gz","dest_dir":"/mnt/r","confirm_name":"etc-20260914-030000.tar.gz"}`
	rec = covRec()
	s.handleBackupRestore(rec, covReq(http.MethodPost, "/r", strings.NewReader(body)), ghostCfg)
	covWantCode(t, "restore offline", rec, http.StatusServiceUnavailable)

	// task get：agent 离线 → 503
	rec = covRec()
	s.handleBackupTaskGet(rec, covReq(http.MethodGet, "/t", nil), ghostCfg, "task-1")
	covWantCode(t, "task offline", rec, http.StatusServiceUnavailable)

	// restore：agent rpc error → 502
	s2 := newBackupTestServer(t)
	withFakeBackupAgent(t, s2, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return nil, "boom"
	})
	cfg2 := covSeedBackupCfg(t, s2, "a1", "etc")
	rec = covRec()
	s2.handleBackupRestore(rec, covReq(http.MethodPost, "/r", strings.NewReader(body)), cfg2)
	covWantCode(t, "restore rpc error", rec, http.StatusBadGateway)

	// task get：agent rpc error → 502
	rec = covRec()
	s2.handleBackupTaskGet(rec, covReq(http.MethodGet, "/t", nil), cfg2, "task-1")
	covWantCode(t, "task rpc error", rec, http.StatusBadGateway)
}

// covFailWriter Write 恒失败的 ResponseWriter（覆盖下载流写失败分支）
type covFailWriter struct{ header http.Header }

func (w *covFailWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}
func (w *covFailWriter) Write([]byte) (int, error) { return 0, errCovWrite }
func (w *covFailWriter) WriteHeader(int)           {}

var errCovWrite = &covWriteError{}

type covWriteError struct{}

func (e *covWriteError) Error() string { return "cov write failure" }

func TestCovBackupDownloadBranches(t *testing.T) {
	// agent 离线 → 503
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	ghostCfg := newBackupCfg("ghost-agent", "ghost", "manual", true)
	if err := s.db.CreateBackupConfig(ghostCfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupFileDownload(rec, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), ghostCfg)
	covWantCode(t, "download offline", rec, http.StatusServiceUnavailable)

	// 首块 decode err（status 非字符串）→ 502
	s2 := newBackupTestServer(t)
	covBadPayloadAgent(t, s2, "a1")
	cfg2 := covSeedBackupCfg(t, s2, "a1", "etc")
	rec = covRec()
	s2.handleBackupFileDownload(rec, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), cfg2)
	covWantCode(t, "download decode error", rec, http.StatusBadGateway)

	// 首块 base64 非法 → 502
	s3 := newBackupTestServer(t)
	withFakeBackupAgent(t, s3, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"data": "!!!not-base64!!!", "size": 10, "eof": true}, ""
	})
	cfg3 := covSeedBackupCfg(t, s3, "a1", "etc")
	rec = covRec()
	s3.handleBackupFileDownload(rec, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), cfg3)
	covWantCode(t, "download bad base64", rec, http.StatusBadGateway)

	// 中途第二块失败 → 已写 200 后静默截断
	s4 := newBackupTestServer(t)
	withFakeBackupAgent(t, s4, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if off, _ := params["offset"].(float64); off == 0 {
			return map[string]interface{}{
				"data": "eA==", // "x"
				"size": 100, "eof": false,
			}, ""
		}
		return nil, "read failed mid-stream"
	})
	cfg4 := covSeedBackupCfg(t, s4, "a1", "etc")
	rec = covRec()
	s4.handleBackupFileDownload(rec, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), cfg4)
	if rec.Code != http.StatusOK || rec.Body.String() != "x" {
		t.Errorf("mid-stream fail: code=%d body=%q", rec.Code, rec.Body.String())
	}

	// 首块写 ResponseWriter 失败 → 直接返回
	s5 := newBackupTestServer(t)
	withFakeBackupAgent(t, s5, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"data": "eA==", "size": 100, "eof": false}, ""
	})
	cfg5 := covSeedBackupCfg(t, s5, "a1", "etc")
	fw := &covFailWriter{}
	s5.handleBackupFileDownload(fw, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), cfg5)
}

// 客户端中途断开（ctx 取消）→ 流静默截断
func TestCovBackupDownloadClientAbort(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if off, _ := params["offset"].(float64); off == 0 {
			return map[string]interface{}{"data": "eA==", "size": 4096, "eof": false}, ""
		}
		time.Sleep(300 * time.Millisecond) // 留出取消窗口
		return map[string]interface{}{"data": "eA==", "size": 4096, "eof": true}, ""
	})
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil).WithContext(ctx)
	rec := covRec()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleBackupFileDownload(rec, req, cfg)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not return after client abort")
	}
}

// ============ CallAgent send timeout（5s，并行摊销）============

func TestCovBackupCallAgentTimeout(t *testing.T) {
	s := newBackupTestServer(t)
	covStuckAgent(t, s, "stuck")
	cfg := covSeedBackupCfg(t, s, "stuck", "etc")

	calls := []struct {
		name string
		run  func()
	}{
		{"files", func() {
			rec := covRec()
			s.handleBackupFiles(rec, covReq(http.MethodGet, "/f", nil), cfg)
			covWantCode(t, "files timeout", rec, http.StatusBadGateway)
		}},
		{"file delete", func() {
			rec := covRec()
			s.handleBackupFileDelete(rec, covReq(http.MethodPost, "/fd", strings.NewReader(`{"name":"etc-20260914-030000.tar.gz"}`)), cfg)
			covWantCode(t, "file delete timeout", rec, http.StatusBadGateway)
		}},
		{"restore", func() {
			rec := covRec()
			s.handleBackupRestore(rec, covReq(http.MethodPost, "/r", strings.NewReader(
				`{"file":"etc-20260914-030000.tar.gz","dest_dir":"/mnt/r","confirm_name":"etc-20260914-030000.tar.gz"}`)), cfg)
			covWantCode(t, "restore timeout", rec, http.StatusBadGateway)
		}},
		{"task get", func() {
			rec := covRec()
			s.handleBackupTaskGet(rec, covReq(http.MethodGet, "/t", nil), cfg, "task-1")
			covWantCode(t, "task get timeout", rec, http.StatusBadGateway)
		}},
		{"download", func() {
			rec := covRec()
			s.handleBackupFileDownload(rec, covReq(http.MethodGet, "/d?name=etc-20260914-030000.tar.gz", nil), cfg)
			covWantCode(t, "download timeout", rec, http.StatusBadGateway)
		}},
	}
	var wg sync.WaitGroup
	for _, c := range calls {
		wg.Add(1)
		go func(c struct {
			name string
			run  func()
		}) {
			defer wg.Done()
			c.run()
		}(c)
	}
	wg.Wait()
}

// ============ listRuns limit 解析补充（经 handleBackupConfigRuns）============

func TestCovBackupRunsLimit(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)
	cfg := covSeedBackupCfg(t, s, "a1", "etc")

	for _, q := range []string{"?limit=5", "?limit=0", "?limit=999", ""} {
		rec := covRec()
		s.handleBackupConfigRuns(rec, covReq(http.MethodGet, "/runs"+q, nil), cfg)
		covWantCode(t, "runs"+q, rec, http.StatusOK)
	}
}
