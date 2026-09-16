package probe

// cov_runner_test.go 覆盖 runner.go 的边界分支：
// Interval/FailThreshold 越界兜底、LoadFailThreshold 空 DB、
// recordHistory/maybePruneHistory 的落库失败日志、TCP 探测失败与
// host:port 兜底解析、探测中途关库的 UpdateServiceStatus 失败日志、
// 以及 Start 循环的完整周期（首轮 10s + 按间隔重复）。

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/cert"
	"github.com/cuihairu/cockpit/internal/health"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

func TestCovIntervalSubSecondFallsBackToDefault(t *testing.T) {
	r, _ := newTestRunner(t)
	// 500ms 会被截断为 0 秒：Interval() 应回退默认 5m
	r2 := NewRunner(r.db, 500*time.Millisecond, nil)
	if got := r2.Interval(); got != 5*time.Minute {
		t.Errorf("Interval() = %v, want 5m", got)
	}
}

func TestCovFailThresholdOutOfRangeFallsBack(t *testing.T) {
	r, _ := newTestRunner(t)
	r.failThreshold.Store(0)
	if got := r.FailThreshold(); got != DefaultFailThreshold {
		t.Errorf("FailThreshold() = %d, want default", got)
	}
	r.failThreshold.Store(99)
	if got := r.FailThreshold(); got != DefaultFailThreshold {
		t.Errorf("FailThreshold() = %d, want default (high out of range)", got)
	}
}

func TestCovLoadFailThresholdNilDB(t *testing.T) {
	r := NewRunner(nil, 0, nil)
	r.LoadFailThreshold() // r.db == nil：直接返回，不 panic
	if got := r.FailThreshold(); got != DefaultFailThreshold {
		t.Errorf("FailThreshold() = %d, want default", got)
	}
}

func TestCovNotifyServiceNilNotifier(t *testing.T) {
	r, _ := newTestRunner(t) // notifier 为 nil
	r.notifyService(ProbeResult{ResourceType: "service", ResourceID: "s1", Name: "svc"}, notification.ServiceDown, "down")
}

func TestCovRecordHistoryClosedDB(t *testing.T) {
	r, db := newTestRunner(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	// 落库失败只记日志不 panic
	r.recordHistory([]ProbeResult{{
		ResourceType: "service",
		ResourceID:   "svc-x",
		Name:         "svc",
		Status:       "down",
		CheckedAt:    time.Now(),
	}})
}

func TestCovMaybePruneHistoryClosedDB(t *testing.T) {
	r, db := newTestRunner(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	// lastPrune 零值距今远超清理间隔：触发清理，失败仅记日志
	r.maybePruneHistory()
}

func TestCovRunAllChecksTCPDown(t *testing.T) {
	r, db := newTestRunner(t)
	if err := db.UpsertService(&storage.Service{ID: "svc-tcp-down", Name: "tcp-down", Type: "tcp", URL: "tcp://127.0.0.1:1", Status: "unknown"}); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}
	results := r.RunAllChecks()
	if results.Services != 1 || results.Unhealthy != 1 {
		t.Fatalf("results = %+v", results)
	}
	got, err := db.GetService("svc-tcp-down")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Status != "down" {
		t.Errorf("status = %q, want down", got.Status)
	}
}

func TestCovProbeServiceHostPortFallback(t *testing.T) {
	r, _ := newTestRunner(t)
	// target 无协议前缀且 host 为空（":1"）：走 host=target、port=80 兜底
	pr := r.probeService(&storage.Service{ID: "svc-colon", Name: ":1", Type: "tcp"})
	if pr.Status != "down" {
		t.Errorf("status = %q, want down", pr.Status)
	}
	if pr.Error == "" {
		t.Error("expected error message for failed TCP probe")
	}
}

func TestCovRunAllChecksUpdateErrorsAfterDBClose(t *testing.T) {
	r, db := newTestRunner(t)

	// 缩短探测超时：让挂起的探测在关库之后才返回，从而触发 Update*Status 失败日志
	r.healthChecker = health.NewChecker(health.Config{Timeout: 2 * time.Second})
	r.certMonitor = cert.NewMonitor(cert.Config{Timeout: 2 * time.Second})

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-block // 挂住服务探测，等待测试关闭数据库
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := db.UpsertService(&storage.Service{ID: "svc-slow", Name: "slow", Type: "http", URL: srv.URL, Status: "unknown"}); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}
	// 192.0.2.1（TEST-NET-1）不可路由：DNS/解析即刻成功，HTTP/TLS 拨号挂起到超时
	if err := db.UpsertDomain(&storage.Domain{ID: "d-slow", Domain: "192.0.2.1", Status: "active"}); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}
	if err := db.UpsertCertificate(&storage.Certificate{ID: "cert-slow", DomainName: "192.0.2.1", Status: "valid", ExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour)}); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	done := make(chan *ProbeResults, 1)
	go func() { done <- r.RunAllChecks() }()

	// 三类 List 均已完成、探测挂起时关库：随后的 Update*Status 失败仅记日志不阻断
	time.Sleep(300 * time.Millisecond)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	close(block)

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("RunAllChecks did not finish after db close")
	}
}

func TestCovStartRunsChecksPeriodically(t *testing.T) {
	r, db := newTestRunner(t)
	if err := db.UpsertService(&storage.Service{ID: "svc-loop", Name: "loop", Type: "http", URL: "http://127.0.0.1:1", Status: "unknown"}); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}
	// 首轮固定 10s 后执行，之后按 Interval() 间隔；压到 1s 让第二轮尽快发生
	r.intervalSeconds.Store(1)

	r.Start()
	defer r.Stop()

	deadline := time.Now().Add(25 * time.Second)
	for {
		rows, err := db.ListProbeResults("service", "svc-loop", 10)
		if err == nil && len(rows) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			rounds := 0
			if rows2, err2 := db.ListProbeResults("service", "svc-loop", 10); err2 == nil {
				rounds = len(rows2)
			}
			t.Fatalf("Start loop did not run twice in time (rounds=%d)", rounds)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ============ probeCertificate 状态三分支（本地 TLS + 自签证书） ============

// covStartTLS 起一个本地 TLS 服务，证书 NotAfter 由参数控制
func covStartTLS(t *testing.T, notAfter time.Time) int {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(cryptorand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: priv}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			conn.Close()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestCovProbeCertificateStatuses(t *testing.T) {
	savePort := probeCertPort
	t.Cleanup(func() { probeCertPort = savePort })
	r := NewRunner(nil, 0, nil)

	cases := []struct {
		name     string
		notAfter time.Time
		want     string
	}{
		{"valid", time.Now().Add(90 * 24 * time.Hour), "valid"},
		{"expiring", time.Now().Add(10 * 24 * time.Hour), "expiring"},
		{"expired", time.Now().Add(-time.Hour), "expired"},
	}
	for _, tc := range cases {
		probeCertPort = covStartTLS(t, tc.notAfter)
		c := &storage.Certificate{ID: "c-" + tc.name, DomainName: "127.0.0.1"}
		pr := r.probeCertificate(c)
		if pr.Status != tc.want {
			t.Errorf("%s: status = %q (msg %q), want %q", tc.name, pr.Status, pr.Message, tc.want)
		}
		if c.ExpiresAt.IsZero() {
			t.Errorf("%s: ExpiresAt should be updated from NotAfter", tc.name)
		}
	}
}
