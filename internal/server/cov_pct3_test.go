package server

// cov_pct3_test.go 覆盖率补测第三批：
//   - 四个 RPC scan（ddns/drift/nas/smart）的 CallAgent 失败、Decode 失败、
//     status error、Unmarshal 失败分支（Marshal(Data) 失败分支不可达，见
//     文件尾注释）
//   - backup_loop / server_backup 的通知与 rclone 摘要截断、列表竞态
//   - recording 远端目标脏值
//   - totp generate 的密钥生成失败与 QR URL 失败
//   - ticket cleanupLoop 过期清理
//   - websocket 注册竞态 rejection
//   - logs follow drain 中写流失败的收尾分支

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// TestCovPctScanCallFail 覆盖四个 scan 函数的 CallAgent 错误分支
// （ddns_scan/drift_scan/nas_scan/smart_scan 各自的 resp, err := CallAgent
// 后 err != nil return）：使用从未注册的 agentID，CallAgent 立即返回
// "agent not found" 类错误。
func TestCovPctScanCallFail(t *testing.T) {
	s := covNewServer(t)

	if _, err := s.fetchAgentIP("cov-missing-agent", "A"); err == nil {
		t.Errorf("fetchAgentIP: want error for missing agent")
	}
	if _, err := s.fetchDriftItems("cov-missing-agent"); err == nil {
		t.Errorf("fetchDriftItems: want error for missing agent")
	}
	if _, err := s.fetchNASSnapshot("cov-missing-agent"); err == nil {
		t.Errorf("fetchNASSnapshot: want error for missing agent")
	}
	if _, err := s.fetchSmartDevices("cov-missing-agent"); err == nil {
		t.Errorf("fetchSmartDevices: want error for missing agent")
	}
}

// TestCovPctScanDecodeFail 覆盖四个 scan 的 DecodeRPCResponse 错误分支：
// 假 agent 应答 payload 的 data 里含 NaN，DecodePayload 内部
// json.Marshal(payload) 对 NaN 报 unsupported value，Decode 失败。
func TestCovPctScanDecodeFail(t *testing.T) {
	s := covNewServer(t)

	covFakeAgent(t, s, "cov-scan-dec", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"x": math.NaN()})
	})

	if _, err := s.fetchAgentIP("cov-scan-dec", "A"); err == nil {
		t.Errorf("fetchAgentIP: want decode error")
	}
	if _, err := s.fetchDriftItems("cov-scan-dec"); err == nil {
		t.Errorf("fetchDriftItems: want decode error")
	}
	if _, err := s.fetchNASSnapshot("cov-scan-dec"); err == nil {
		t.Errorf("fetchNASSnapshot: want decode error")
	}
	if _, err := s.fetchSmartDevices("cov-scan-dec"); err == nil {
		t.Errorf("fetchSmartDevices: want decode error")
	}
}

// TestCovPctScanStatusError 覆盖四个 scan 的 rpcResp.Status == "error"
// 分支：假 agent 应答 covErrPayload，scan 把 agent 错误透传成 error。
func TestCovPctScanStatusError(t *testing.T) {
	s := covNewServer(t)

	covFakeAgent(t, s, "cov-scan-err", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("scan refused")
	})

	if _, err := s.fetchAgentIP("cov-scan-err", "A"); err == nil {
		t.Errorf("fetchAgentIP: want status error")
	}
	if _, err := s.fetchDriftItems("cov-scan-err"); err == nil {
		t.Errorf("fetchDriftItems: want status error")
	}
	if _, err := s.fetchNASSnapshot("cov-scan-err"); err == nil {
		t.Errorf("fetchNASSnapshot: want status error")
	}
	if _, err := s.fetchSmartDevices("cov-scan-err"); err == nil {
		t.Errorf("fetchSmartDevices: want status error")
	}
}

// TestCovPctScanUnmarshalFail 覆盖四个 scan 的 json.Unmarshal 错误分支：
// data 是合法 JSON 字符串（非对象），Marshal(Data) 成功但反序列化到结构体
// 报 UnmarshalTypeError。
func TestCovPctScanUnmarshalFail(t *testing.T) {
	s := covNewServer(t)

	covFakeAgent(t, s, "cov-scan-bad", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload("not-an-object")
	})

	if _, err := s.fetchAgentIP("cov-scan-bad", "A"); err == nil {
		t.Errorf("fetchAgentIP: want unmarshal error")
	}
	if _, err := s.fetchDriftItems("cov-scan-bad"); err == nil {
		t.Errorf("fetchDriftItems: want unmarshal error")
	}
	if _, err := s.fetchNASSnapshot("cov-scan-bad"); err == nil {
		t.Errorf("fetchNASSnapshot: want unmarshal error")
	}
	if _, err := s.fetchSmartDevices("cov-scan-bad"); err == nil {
		t.Errorf("fetchSmartDevices: want unmarshal error")
	}
}

// TestCovPctNotifyBackupRemoteFailedEmptyMsg 覆盖 backup_loop.go 的
// notifyBackupRemoteFailed：errMsg 为空时改写为默认文案并调用
// SendNonBlocking（notifier 为 NewService(nil)，无通道，安全空转）。
func TestCovPctNotifyBackupRemoteFailedEmptyMsg(t *testing.T) {
	s := covNewServer(t)

	s.notifyBackupRemoteFailed(&storage.BackupConfig{Name: "cov-bak"}, "")
}

// TestCovPctServerBackupNotifyCovers 覆盖 server_backup.go 的
// notifyServerBackupRemoteFailed 两条路径：notifier 非 nil 走
// SendNonBlocking；直构 &Server{}（notifier nil）走提前 return。
func TestCovPctServerBackupNotifyCovers(t *testing.T) {
	s := covNewServer(t)
	s.notifyServerBackupRemoteFailed("cockpit-20200101-000000.db", "rclone boom")

	bare := &Server{}
	bare.notifyServerBackupRemoteFailed("cockpit-20200101-000000.db", "rclone boom")
}

// TestCovPctRecordingRemoteDestInvalid 覆盖 recording.go 的脏值分支：
// remote_dest 写入口之后被手改成含空格的非法值，recordingRemoteDest
// 视为未配置返回空串并记日志。
func TestCovPctRecordingRemoteDestInvalid(t *testing.T) {
	s := covNewServer(t)

	if err := s.db.SetSetting(RecordingRemoteDestSettingKey, "my remote:bak"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	if got := s.recordingRemoteDest(); got != "" {
		t.Errorf("recordingRemoteDest = %q, want empty for invalid value", got)
	}
}

// TestCovPctRcloneCopyTrims 覆盖 server_backup.go 的 rcloneCopyLocalFile
// 长输出截断分支：伪 rclone 输出 3000 字节后 exit 1，使 summary 超过
// 2048（走 summary 截断 + 日志）、detail 超过 512（走 detail 截断并拼入
// 错误信息）。
func TestCovPctRcloneCopyTrims(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "fake-rclone")
	script := "#!/bin/sh\nhead -c 3000 /dev/zero | tr '\\0' 'x'\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rclone: %v", err)
	}
	setServerRclone(t, bin, 5*time.Second)

	local := filepath.Join(t.TempDir(), "src.db")
	if err := os.WriteFile(local, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	err := rcloneCopyLocalFile(local, "remote:bak")
	if err == nil {
		t.Fatalf("rcloneCopyLocalFile: want error, got nil")
	}
	if !strings.Contains(err.Error(), "rclone copy") {
		t.Errorf("error = %q, want rclone copy prefix", err)
	}
}

// TestCovPctServerBackupListInfoRace 覆盖 server_backup.go 的
// listServerBackups 中 e.Info() 失败 continue 分支：并发 rename 备份目录，
// 使 ReadDir 返回的 entry 在 Info()（按路径 Lstat）时目录已被挪走，
// Lstat 报错进入 continue。这是该分支唯一可观测的触发手段（文件删除
// 竞态窗口过窄）。
func TestCovPctServerBackupListInfoRace(t *testing.T) {
	s := covNewServer(t)
	s.cfg = &config.Config{Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "cockpit.db")}}

	dir := s.serverBackupDir()
	// 目录尚未创建：覆盖 listServerBackups 的 os.IsNotExist 空列表分支
	if list, err := s.listServerBackups(); err != nil || len(list) != 0 {
		t.Fatalf("listServerBackups on missing dir: list=%v err=%v", list, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 放一个子目录与一个非法名文件：覆盖 e.IsDir()/非法名 continue 分支
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-backup.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write invalid name: %v", err)
	}
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("cockpit-20200101-%06d.db", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed backup file: %v", err)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		swap := dir + ".swap"
		for {
			select {
			case <-stop:
				_ = os.Rename(swap, dir) // 收尾还原，保证清理路径完整
				return
			default:
				_ = os.Rename(dir, swap)
				_ = os.Rename(swap, dir)
			}
		}
	}()

	for i := 0; i < 600; i++ {
		if _, err := s.listServerBackups(); err != nil {
			t.Fatalf("listServerBackups: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

// TestCovPctTOTPGenerateBadUsername 覆盖 totp_handlers.go 的两处 500：
//   - Username 为空时 pquerna/otp Generate 报 ErrGenerateMissingAccountName，
//     走 GenerateTOTPSecret 失败分支；
//   - Username 含非法百分号转义 "%zz" 时 GenerateTOTPURL 内部
//     NewKeyFromURL 的 url.Parse 失败，走 QR URL 失败分支。
//
// 两个子测试均到达不了 totpTmpStore 写点，不会与其它测试竞争全局 map。
func TestCovPctTOTPGenerateBadUsername(t *testing.T) {
	t.Run("empty username", func(t *testing.T) {
		s := newTestServerWithDB(t)
		u := &storage.User{Username: "", Role: "user", Password: "x"}
		if err := s.db.CreateUser(u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		req := covAuthReq("POST", "/api/auth/totp/generate", nil, u.ID, "", "user")
		rec := covCallAuth(s, s.handleTOTPGenerate, req)
		covWantCode(t, "empty username generate", rec, http.StatusInternalServerError)
	})

	t.Run("bad percent username", func(t *testing.T) {
		s := newTestServerWithDB(t)
		u := &storage.User{Username: "cov%zz", Role: "user", Password: "x"}
		if err := s.db.CreateUser(u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		req := covAuthReq("POST", "/api/auth/totp/generate", nil, u.ID, "cov%zz", "user")
		rec := covCallAuth(s, s.handleTOTPGenerate, req)
		covWantCode(t, "bad percent generate", rec, http.StatusInternalServerError)
	})
}

// TestCovPctTicketCleanupExpires 覆盖 ticket.go cleanupLoop 的过期删除
// 分支：ticker 硬编码 1 分钟，测试塞入一张已过期票据与一张有效票据后
// 启动 cleanupLoop，持锁轮询断言过期票被删、有效票保留（约 61s，
// 轮询 + deadline，无固定睡眠时序假设）。
func TestCovPctTicketCleanupExpires(t *testing.T) {
	tm := &TicketManager{tickets: make(map[string]*Ticket)}

	tm.mu.Lock()
	tm.tickets["expired-1"] = &Ticket{ID: "expired-1", ExpiresAt: time.Now().Add(-time.Minute)}
	tm.tickets["alive-1"] = &Ticket{ID: "alive-1", ExpiresAt: time.Now().Add(time.Hour)}
	tm.mu.Unlock()

	go tm.cleanupLoop()

	deadline := time.Now().Add(90 * time.Second)
	for {
		tm.mu.Lock()
		_, hasExpired := tm.tickets["expired-1"]
		_, hasAlive := tm.tickets["alive-1"]
		tm.mu.Unlock()

		if !hasExpired {
			if !hasAlive {
				t.Fatalf("cleanup removed the still-valid ticket")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanupLoop did not remove expired ticket within deadline")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestCovPctWSRegistrationRace 覆盖 websocket.go 的 registration_failed
// rejection。原实现赌两个 goroutine「都越过 db 查重、在 registry.Register
// 处碰撞」的极窄窗口——高负载（CI race 检测）下后到者几乎总被
// registry.Get 先拦成 duplicate_connection，15s 可能零命中（CI 偶发
// FAIL）。改为构造性验证：先注册者正常成功；后注册者注入跳过 Get
// 检查（即"检查时不存在、注册时已被抢先"的 TOCTOU 后半程），确定性
// 撞 ErrAgentAlreadyExists → registration_failed。
func TestCovPctWSRegistrationRace(t *testing.T) {
	s := newTestServerWithDB(t)

	reg := &protocol.RegisterPayload{AgentID: "cov-ws-race-a", Hostname: "host-cov-ws-race-a"}
	if _, rejection, err := s.registerAgentConnection(nil, reg); err != nil || rejection != nil {
		t.Fatalf("first registration: err=%v rejection=%v", err, rejection)
	}

	orig := wsRegistryLookup
	wsRegistryLookup = func(*Registry, string) (*Agent, bool) { return nil, false }
	defer func() { wsRegistryLookup = orig }()

	reg2 := &protocol.RegisterPayload{AgentID: "cov-ws-race-a", Hostname: "host-cov-ws-race-a"}
	_, rejection, err := s.registerAgentConnection(nil, reg2)
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if rejection == nil || rejection.code != "registration_failed" {
		t.Fatalf("expected registration_failed, got %+v", rejection)
	}
}

// covPctFollowWriter 流式响应 writer：前 failAfter 次 Write 成功，其后
// 返回错误（模拟客户端断开）。secondWrite 在第 2 次成功写时发信号，且
// 第 3 次写停顿 50ms，为测试在「主循环回到 select 之前」完成喂行 +
// close 制造宽松窗口。Flush 为空实现满足 http.Flusher 断言。
type covPctFollowWriter struct {
	failAfter int
	writes    int
	second    chan struct{}
}

func (w *covPctFollowWriter) Header() http.Header { return http.Header{} }

func (w *covPctFollowWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == 2 && w.second != nil {
		select {
		case w.second <- struct{}{}:
		default:
		}
	}
	if w.writes == 3 {
		time.Sleep(50 * time.Millisecond) // 撑宽窗口：测试端此刻喂第 4 行 + close
	}
	if w.writes > w.failAfter {
		return 0, errors.New("client gone")
	}
	return len(p), nil
}

func (w *covPctFollowWriter) WriteHeader(int) {}
func (w *covPctFollowWriter) Flush()          {}

// TestCovPctLogsFollowDrainWriteFail 覆盖 api_logs_follow.go 的 drain
// 分支写失败收尾（case reason := <-f.done 内层循环中 writeFrame 失败
// return）：假 agent 捕获 followId；writer failAfter=3，先喂 3 行，writer
// 在写第 2 行时发信号、写第 3 行时停顿 50ms，测试端在停顿窗口内喂
// 第 4 行并 close——主循环随后回到 select 时「第 4 行」与「done」均已
// 就绪，随机二选一：选中 done 进 drain 后消费第 4 行即写失败命中目标
// 行（单轮约 50%，固定跑满 30 轮后仍全不命中的概率约 2^-30，工程上
// 等价确定；50ms 停顿是主动制造的竞速窗口，不是对固定时序的依赖）。
// 走 drain 还是主循环失败在 writer 侧不可观测（两者 Write 序列相同），
// 以 coverprofile 作为最终判据，这里仅统计失败写次数防整轮空转。
func TestCovPctLogsFollowDrainWriteFail(t *testing.T) {
	s := newTestServerWithDB(t)

	var mu sync.Mutex
	var followID string
	started := make(chan struct{}, 1)
	covFakeAgent(t, s, "cov-logf-agent", []string{"logs"}, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "logs.follow.start" {
			mu.Lock()
			followID, _ = params["followId"].(string)
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			return covOKPayload(nil)
		}
		// follow.stop 等其余调用也立即应答，避免 CallAgent 30s 超时挂住收尾
		return covOKPayload(nil)
	})

	hit := 0
	for round := 0; round < 30; round++ {
		req := httptest.NewRequest("POST", "/api/agents/cov-logf-agent/logs/follow",
			strings.NewReader(`{"type":"systemd","source":"cockpit","tail":1}`))
		w := &covPctFollowWriter{failAfter: 3, second: make(chan struct{}, 1)}
		handlerDone := make(chan struct{})
		go func() {
			defer close(handlerDone)
			s.handleAgentLogsFollow(w, req, "cov-logf-agent")
		}()

		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("follow start not observed")
		}
		mu.Lock()
		fid := followID
		mu.Unlock()

		// 喂 3 行；writer 写第 2 行时发信号，测试端随即在第 3 行的写停顿
		// 窗口内完成第 4 行 + close 的投递
		for i := 0; i < 3; i++ {
			s.HandleLogsFollowData("logs:"+fid, []byte(fmt.Sprintf("line-%d", i)))
		}
		select {
		case <-w.second:
		case <-time.After(5 * time.Second):
			t.Fatalf("handler did not consume 2 lines (writes=%d)", w.writes)
		}
		s.HandleLogsFollowData("logs:"+fid, []byte("line-3"))
		s.HandleLogsFollowClose("logs:"+fid, "exited")

		select {
		case <-handlerDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("handler did not finish")
		}
		if w.writes > 3 {
			hit++ // 发生过失败写（drain 内或主循环内，最终以 coverprofile 区分）
		}
	}
	if hit == 0 {
		t.Fatalf("write-failure path not observed in 30 rounds")
	}
	t.Logf("write-failure rounds: %d/30 (drain share expected ~50%%, verify via coverprofile)", hit)
}

// TestCovPctBackupFileDownloadRoute 覆盖 api_backups.go 的
// configs/{id}/files/download 路由 case 匹配行与 requireAgentRclone 的
// agent 缺失放行分支：配置指向不存在 agent，requireAgentRclone 返回
// true（报错交给调用方），随后 download 的 CallAgent 失败收尾。
func TestCovPctBackupFileDownloadRoute(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := &storage.BackupConfig{AgentID: "bak-missing-agent", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupsAPI(rec, covReq(http.MethodGet, "/api/backups/configs/1/files/download", nil))
	if rec.Code == http.StatusNotImplemented {
		t.Errorf("download route case not matched: %d", rec.Code)
	}
}

// TestCovPctAgentNASBadPath 覆盖 api_nas.go 的 handleAgentNASAPI 路径
// 缺少 /nas/ 后缀时的 404 分支（idx <= 0）。
func TestCovPctAgentNASBadPath(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/a1/nope", nil), "no-suffix-here")
	covWantCode(t, "nas bad path 404", rec, http.StatusNotFound)
}

// TestCovPctOverlayTSDeviceEmptyID 覆盖 api_overlay_cloud.go 的
// handleOverlayTSDevice 空设备 ID 404 分支。
func TestCovPctOverlayTSDeviceEmptyID(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleOverlayTSDevice(rec, covReq(http.MethodGet, "/api/overlay/tailscale/devices/", nil), "/")
	covWantCode(t, "ts device empty id 404", rec, http.StatusNotFound)
}

// TestCovPctAcmeIssueObtainFail 覆盖 acme_issuer.go Issue 的
// Certificate.Obtain 错误分支：把包级 acmeDirectoryURLs 的 staging 条目
// 临时改指本测试的 HTTPS fake（同包测试注入，t.Cleanup 还原），fake 的
// newAccount 返回 201、newOrder 落 default 分支返回 200 {}，lego 解析
// 空 order 后在 Obtain 内部失败，全程不出网。
func TestCovPctAcmeIssueObtainFail(t *testing.T) {
	db := testServerDB(t)
	fakeURL := covPctFakeACME(t, 0)

	old := acmeDirectoryURLs[AcmeCADirectoryStaging]
	acmeDirectoryURLs[AcmeCADirectoryStaging] = fakeURL
	t.Cleanup(func() { acmeDirectoryURLs[AcmeCADirectoryStaging] = old })

	// dnsProvider 需 Ready() 通过才能走到 Obtain：给足 cloudflare token
	l := NewLegoIssuer(db, func() AcmeDNSConfig {
		return AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "cov-token"}
	}).(*legoIssuer)

	_, err := l.Issue(&storage.AcmeCert{
		Domains:       []string{"cov.example.com"},
		PrimaryDomain: "cov.example.com",
		CADirectory:   AcmeCADirectoryStaging,
	})
	if err == nil {
		t.Fatalf("Issue: want obtain error, got nil")
	}
}

// covPctFakeACMEOrderReject 与 cov_pct_test.go 的 covPctFakeACME 同骨架的
// HTTPS ACME 伪造端点（covPctFakeACME 无法改签名，此处独立成变体）：
// newAccount 返回 201，newOrder 返回 500 + ACME problem JSON，使
// Certificate.Obtain 在下单处失败（不触碰 DNS 挑战，全程不出网）。
func covPctFakeACMEOrderReject(t *testing.T) string {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "https://" + r.Host
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"newNonce": %q, "newAccount": %q, "newOrder": %q,
				"revokeCert": %q, "keyChange": %q
			}`, base+"/nonce", base+"/new-account", base+"/new-order",
				base+"/revoke-cert", base+"/key-change")))
		case "/nonce":
			w.Header().Set("Replay-Nonce", "cov-nonce")
			w.WriteHeader(http.StatusNoContent)
		case "/new-account":
			w.Header().Set("Location", base+"/account/1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"valid"}`))
		case "/new-order":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"type":"urn:ietf:params:acme:error:serverInternal","detail":"cov order rejected"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	})

	// 自建 IsCA=true 的 CA 并签发 SAN 含 127.0.0.1/::1/localhost 的服务端
	// 证书（同 covPctFakeACME，httptest 自签证书无法作为根加入证书池）
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cov-pct-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen server key: %v", err)
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{srvDER, caDER},
		PrivateKey:  srvKey,
	}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := filepath.Join(t.TempDir(), "cov-acme-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write ca pem: %v", err)
	}
	t.Setenv("LEGO_CA_CERTIFICATES", caFile)
	return srv.URL
}

// TestCovPctAcmeIssueOrderReject 覆盖 acme_issuer.go Issue 的
// Certificate.Obtain 错误分支：注入包级 acmeDirectoryURLs 指向 fake，
// newOrder 返回 500 problem JSON，lego 在 Obtain 内下单失败返回错误。
func TestCovPctAcmeIssueOrderReject(t *testing.T) {
	db := testServerDB(t)
	fakeURL := covPctFakeACMEOrderReject(t)

	old := acmeDirectoryURLs[AcmeCADirectoryStaging]
	acmeDirectoryURLs[AcmeCADirectoryStaging] = fakeURL
	t.Cleanup(func() { acmeDirectoryURLs[AcmeCADirectoryStaging] = old })

	l := NewLegoIssuer(db, func() AcmeDNSConfig {
		return AcmeDNSConfig{Provider: "cloudflare", CloudflareToken: "cov-token"}
	}).(*legoIssuer)

	_, err := l.Issue(&storage.AcmeCert{
		Domains:       []string{"cov.example.com"},
		PrimaryDomain: "cov.example.com",
		CADirectory:   AcmeCADirectoryStaging,
	})
	if err == nil || !strings.Contains(err.Error(), "acme obtain") {
		t.Fatalf("Issue: want acme obtain error, got %v", err)
	}
}

// TestCovPctBackupFileSyncRemoteRoute 覆盖 api_backups.go 的 sync-remote
// 路由 case 匹配行（既有 sync 测试均直调 handler 未走路由）：目标 agent
// 不存在，handler 内的在线检查报 503。
func TestCovPctBackupFileSyncRemoteRoute(t *testing.T) {
	s := newBackupTestServer(t)
	cfg := &storage.BackupConfig{AgentID: "bak-route-missing", Name: "bak",
		Sources: marshalSources([]string{"/etc"}), DestDir: "/b", Schedule: "manual",
		RemoteDest: "r:bak"}
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rec := covRec()
	s.handleBackupsAPI(rec, covReq(http.MethodPost, "/api/backups/configs/1/files/sync-remote",
		strings.NewReader(`{"name":"20240101-000000.node1.tar.gz"}`)))
	covWantCode(t, "sync-remote via route 503", rec, http.StatusServiceUnavailable)
}

// TestCovPctBackupCreateRcloneAgentMissing 覆盖 requireAgentRclone 的
// agent 缺失放行分支（api_backups.go L183-185）：create 前置的存在性
// Get 与 requireAgentRclone 内的第二次 Get 背靠背，只有 toggle goroutine
// 在两次 Get 之间恰好 Unregister 才能进入放行 return true（创建成功
// 200）。toggle 周期性 Register/Unregister 同一 agent，主 goroutine
// 轮询 create 请求直至命中（15s deadline，与 cov_pct2 竞态手法同源）。
func TestCovPctBackupCreateRcloneAgentMissing(t *testing.T) {
	s := newBackupTestServer(t)

	const agentID = "bak-race-rclone"
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			agent := NewAgent(agentID, nil)
			agent.Capabilities = []protocol.Capability{{
				Type:     "backup",
				Metadata: map[string]interface{}{"rclone": true},
			}}
			if err := s.registry.Register(agent); err != nil {
				continue
			}
			s.registry.Unregister(agentID)
		}
	}()
	deadline := time.Now().Add(15 * time.Second)
	hit := false
	for n := 0; !hit && time.Now().Before(deadline); n++ {
		body := fmt.Sprintf(`{"agent_id":%q,"name":"bak-%d","sources":["/etc"],"dest_dir":"/b","schedule":"manual","remote_dest":"r:bak"}`,
			agentID, n)
		rec := covRec()
		s.handleBackupsAPI(rec, covReq(http.MethodPost, "/api/backups/configs",
			strings.NewReader(body)))
		if rec.Code == http.StatusCreated {
			hit = true
		}
	}
	close(stop)
	wg.Wait()
	if !hit {
		t.Fatalf("requireAgentRclone missing-agent pass-through not observed within deadline")
	}
}
