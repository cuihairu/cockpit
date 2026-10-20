package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func livenessFixture() []livenessTargetPayload {
	return []livenessTargetPayload{
		{
			IP:   "10.0.0.5",
			ARP:  livenessSourcePayload{Verdict: "present", Detail: "arp complete aa:bb:cc:dd:ee:ff on br-lan"},
			ICMP: livenessSourcePayload{Verdict: "unknown"},
			TCP:  livenessSourcePayload{Verdict: "present", Detail: "tcp 22 connected"},
			At:   1760000000,
		},
		{IP: "10.0.0.6", ICMP: livenessSourcePayload{Verdict: "unknown"}},
		{IP: "", ARP: livenessSourcePayload{Verdict: "present"}}, // 空 IP 必须被丢弃
	}
}

// TestLivenessStoreRecordAndQuery 记录与过滤查询：按 agent/ip 过滤，按到达
// 时刻与 IP 排序，空 IP 不入表。
func TestLivenessStoreRecordAndQuery(t *testing.T) {
	st := newLivenessStore()
	base := time.Unix(1760000000, 0)
	st.now = func() time.Time { return base }
	st.Record("a1", livenessFixture())
	// 跨秒记录，覆盖排序的到达时刻分支
	st.now = func() time.Time { return base.Add(time.Second) }
	st.Record("a2", []livenessTargetPayload{{IP: "10.0.0.7"}})
	st.now = func() time.Time { return base }

	all := st.Query("", "")
	if len(all) != 3 {
		t.Fatalf("entries = %d, want 3", len(all))
	}
	if all[0].Report.IP != "10.0.0.5" || all[0].SeenAt != 1760000000 ||
		all[0].AgeSeconds != 0 || all[0].Stale {
		t.Fatalf("first entry = %+v", all[0])
	}
	// 同秒条目按 IP 兜底排序
	if all[1].Report.IP != "10.0.0.6" || all[1].SeenAt != 1760000000 {
		t.Fatalf("same-second order = %+v", all)
	}

	if got := st.Query("a1", ""); len(got) != 2 {
		t.Fatalf("a1 entries = %+v", got)
	}
	if got := st.Query("", "10.0.0.6"); len(got) != 1 || got[0].Report.IP != "10.0.0.6" {
		t.Fatalf("ip entries = %+v", got)
	}
	if got := st.Query("a1", "10.0.0.7"); len(got) != 0 {
		t.Fatalf("mismatched filter = %+v", got)
	}
}

// TestLivenessStoreUpsertAndStale 重复上报刷新到达时刻；超过门槛标 stale。
func TestLivenessStoreUpsertAndStale(t *testing.T) {
	st := newLivenessStore()
	base := time.Unix(1000, 0)
	st.now = func() time.Time { return base }
	st.Record("a1", []livenessTargetPayload{{IP: "10.0.0.5", ARP: livenessSourcePayload{Verdict: "present"}}})

	st.now = func() time.Time { return base.Add(time.Second) }
	st.Record("a1", []livenessTargetPayload{{IP: "10.0.0.5", ARP: livenessSourcePayload{Verdict: "unknown"}}})
	entry := st.Query("a1", "")[0]
	if entry.SeenAt != base.Add(time.Second).Unix() || entry.Report.ARP.Verdict != "unknown" {
		t.Fatalf("upsert = %+v", entry)
	}

	st.now = func() time.Time { return base.Add(st.maxAge + 2*time.Second) }
	entry = st.Query("a1", "")[0]
	if !entry.Stale || entry.AgeSeconds <= 0 {
		t.Fatalf("stale = %+v", entry)
	}

	// 远未来时刻（时钟回拨）不得产生负 age
	st.now = func() time.Time { return base.Add(-time.Hour) }
	if got := st.Query("a1", "")[0]; got.AgeSeconds < 0 {
		t.Fatalf("age = %d", got.AgeSeconds)
	}
}

// TestDecodeLivenessReport payload 缺失与坏类型。
func TestDecodeLivenessReport(t *testing.T) {
	p, err := decodeLivenessReport(&protocol.Message{Payload: nil})
	if err != nil || len(p.Reports) != 0 {
		t.Fatalf("nil payload = %+v err=%v", p, err)
	}

	p, err = decodeLivenessReport(&protocol.Message{Payload: map[string]interface{}{"reports": "bad"}})
	if err == nil {
		t.Fatalf("bad payload accepted: %+v", p)
	}

	p, err = decodeLivenessReport(&protocol.Message{Payload: map[string]interface{}{
		"reports": []map[string]interface{}{{"ip": "10.0.0.5", "arp": map[string]interface{}{"verdict": "present"}}},
	}})
	if err != nil || len(p.Reports) != 1 || p.Reports[0].IP != "10.0.0.5" {
		t.Fatalf("decode = %+v err=%v", p, err)
	}
}

// TestHandleLivenessReport 空 store 守卫、正常入库与坏负载容错。
func TestHandleLivenessReport(t *testing.T) {
	s := &Server{}
	s.handleLivenessReport(NewAgent("a1", nil), &protocol.Message{Type: protocol.MessageTypeLivenessReport})

	s = &Server{livenessStore: newLivenessStore()}
	s.handleLivenessReport(NewAgent("a1", nil),
		&protocol.Message{Payload: map[string]interface{}{"reports": livenessFixture()}})
	if got := s.livenessStore.Query("a1", ""); len(got) != 2 {
		t.Fatalf("ingest = %+v", got)
	}
	s.handleLivenessReport(NewAgent("a1", nil), &protocol.Message{Payload: map[string]interface{}{"reports": 5}})
}

// TestHandleOobLiveness 直调 + 走 mux 的鉴权/RBAC 路径（GET 归 inventory:read）。
func TestHandleOobLiveness(t *testing.T) {
	empty := &Server{}
	rec := covRec()
	empty.handleOobLiveness(rec, covReq(http.MethodGet, "/api/oob/liveness", nil))
	covWantCode(t, "nil store", rec, http.StatusServiceUnavailable)

	s := newTestServerWithDB(t)
	s.livenessStore = newLivenessStore()
	s.livenessStore.Record("a1", livenessFixture())

	rec = covRec()
	s.handleOobLiveness(rec, covReq(http.MethodGet, "/api/oob/liveness?ip=10.0.0.6", nil))
	covWantCode(t, "direct query", rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"ip":"10.0.0.6"`) || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("body = %s", rec.Body.String())
	}

	mux := http.NewServeMux()
	s.registerOobAPI(mux)
	rec = covRec()
	mux.ServeHTTP(rec, covReq(http.MethodGet, "/api/oob/liveness?agent_id=a1", nil))
	covWantCode(t, "no auth", rec, http.StatusUnauthorized)

	token, err := s.authService().GenerateToken("1", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := covReq(http.MethodGet, "/api/oob/liveness?agent_id=a1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "with auth", rec, http.StatusOK)
	if strings.Count(rec.Body.String(), `"ip"`) != 2 {
		t.Fatalf("agent filter = %s", rec.Body.String())
	}

	// 探活读接口不审计（纯读，无库存变更）：请求前后审计行数不变
	logs, _, err := s.db.GetAuditLogs(0, 500, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	req = covReq(http.MethodGet, "/api/oob/liveness", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = covRec()
	mux.ServeHTTP(rec, req)
	covWantCode(t, "with auth", rec, http.StatusOK)
	after, _, err := s.db.GetAuditLogs(0, 500, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(logs) {
		t.Fatalf("read must not be audited: %d -> %d", len(logs), len(after))
	}
}

// TestHandleMessageLivenessReport 分发器把 liveness_report 路由到内存最新态。
func TestHandleMessageLivenessReport(t *testing.T) {
	s := &Server{livenessStore: newLivenessStore()}
	s.handleMessage(NewAgent("a9", nil), &protocol.Message{
		Type:    protocol.MessageTypeLivenessReport,
		Payload: map[string]interface{}{"reports": []livenessTargetPayload{{IP: "10.0.0.9"}}},
	})
	if got := s.livenessStore.Query("a9", ""); len(got) != 1 || got[0].Report.IP != "10.0.0.9" {
		t.Fatalf("dispatch = %+v", got)
	}
}
