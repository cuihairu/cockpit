package probe

// cov_io_gaps_test.go 覆盖 RunAllChecks 结果汇总的 Healthy++ 分支与
// else-if 条件链：证书探测 "valid"（本地 TLS + 远期自签证书）计入
// healthy；域名 localhost DNS 可解析但 80/443 无人监听 → degraded
// （非 healthy）使条件链被完整求值。

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestCovRunAllChecksCounts(t *testing.T) {
	r, db := newTestRunner(t)

	// 有效证书：本地 TLS（NotAfter 90 天后）→ "valid" → Healthy++
	savePort := probeCertPort
	probeCertPort = covStartTLS(t, time.Now().Add(90*24*time.Hour))
	t.Cleanup(func() { probeCertPort = savePort })
	if err := db.UpsertCertificate(&storage.Certificate{
		ID: "cov-cert", DomainName: "127.0.0.1", Status: "valid",
		ExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	// degraded 域名：DNS 通、HTTP 不通 → 条件链 else-if 求值
	if err := db.UpsertDomain(&storage.Domain{ID: "d-cov", Domain: "localhost", Status: "active"}); err != nil {
		t.Fatalf("UpsertDomain: %v", err)
	}

	results := r.RunAllChecks()
	if results == nil {
		t.Fatal("nil results")
	}
	if results.Certificates != 1 {
		t.Fatalf("certificate probes = %d, want 1", results.Certificates)
	}
	if results.Healthy != 1 {
		t.Errorf("healthy = %d, want 1 (valid certificate)", results.Healthy)
	}

	for _, res := range results.Results {
		if res.ResourceType == "domain" && res.Status != "degraded" {
			t.Logf("localhost status = %q (env-dependent)", res.Status)
		}
	}
}
