package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ P2 ③ inventory 联动：stack 端口 → CMDB Service 台账行 ============

// stackItem 构造一个带端口上报的 stack 条目（端口值走 JSON 浮点路径）
func stackItem(stack string, ports ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name": stack,
		"services": []interface{}{
			map[string]interface{}{"name": stack + "-svc", "ports": ports},
		},
	}
}

func stackPort(hostPort, containerPort int, proto string) map[string]interface{} {
	return map[string]interface{}{"hostPort": float64(hostPort), "containerPort": float64(containerPort), "proto": proto}
}

// 只收 tcp 已发布端口：udp 与未发布（hostPort=0）都不建行；ID/URL/标签定型
func TestStackServiceSyncCreatesRows(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"

	item := stackItem("web",
		stackPort(8080, 80, "tcp"),
		stackPort(0, 9090, "tcp"),  // 未发布：容器网络内可达，probe 触达不到
		stackPort(5353, 53, "udp"), // udp：probe 仅 TCP/HTTP 探测
	)
	s.cacheStack("a1", item)

	services, err := s.db.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("services = %+v, want 1 row", services)
	}
	got := services[0]
	if got.ID != "stackport-a1-web-8080-tcp" || got.Name != "web-8080" ||
		got.Type != "tcp" || got.URL != "tcp://10.0.0.5:8080" {
		t.Errorf("row = %+v", got)
	}
	if want := map[string]string{"source": "stack", "agent_id": "a1", "stack": "web"}; !reflect.DeepEqual(got.Labels, want) {
		t.Errorf("labels = %v, want %v", got.Labels, want)
	}
	if got.AgentID == nil || *got.AgentID != "a1" {
		t.Errorf("agentId = %v, want a1", got.AgentID)
	}
}

// 台账行是寻址事实源，探测状态归 probe：重同步刷新 URL 但不清 status
func TestStackServiceSyncPreservesProbeState(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"
	s.cacheStack("a1", stackItem("web", stackPort(8080, 80, "tcp")))

	id := "stackport-a1-web-8080-tcp"
	if err := s.db.UpdateServiceStatus(id, "up", 12, time.Now()); err != nil {
		t.Fatal(err)
	}
	// agent IP 变更 → 探测目标刷新，探测状态保留
	a.IP = "10.0.0.9"
	s.cacheStack("a1", stackItem("web", stackPort(8080, 80, "tcp")))

	got, err := s.db.GetService(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "up" || got.ResponseTimeMs != 12 || got.URL != "tcp://10.0.0.9:8080" {
		t.Errorf("row = %+v, want status=up url=tcp://10.0.0.9:8080", got)
	}
}

// 端口对账：本轮集合里没有的台账行删除；其他 stack/其他 agent 的行不动
func TestStackServiceSyncPortDiff(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"
	s.cacheStack("a1", stackItem("web", stackPort(8080, 80, "tcp")))

	a2 := covFakeAgent(t, s, "a2", nil, nil)
	a2.IP = "10.0.0.6"
	s.cacheStack("a2", stackItem("web", stackPort(8081, 80, "tcp")))

	// 端口下线换端口
	s.cacheStack("a1", stackItem("web", stackPort(9090, 80, "tcp")))

	services, err := s.db.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, svc := range services {
		ids[svc.ID] = true
	}
	if !ids["stackport-a1-web-9090-tcp"] || ids["stackport-a1-web-8080-tcp"] {
		t.Fatalf("ids = %v", ids)
	}
	if !ids["stackport-a2-web-8081-tcp"] {
		t.Fatalf("other agent row wiped: %v", ids)
	}
}

// stack 消失对账：全量列表不再上报的 stack 台账行清除；
// 非 stack 来源行（inventory 声明/手工登记）永不触碰
func TestStackServicePruneVanishedStack(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"
	s.cacheStack("a1", stackItem("web", stackPort(8080, 80, "tcp")))
	s.cacheStack("a1", stackItem("db", stackPort(5432, 5432, "tcp")))
	a2 := covFakeAgent(t, s, "a2", nil, nil)
	a2.IP = "10.0.0.6"
	s.cacheStack("a2", stackItem("web", stackPort(8081, 80, "tcp")))
	// inventory 声明行（无 source 标签）+ 另一来源台账行
	if err := s.db.UpsertService(&storage.Service{ID: "manual-1", Name: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertService(&storage.Service{
		ID: "svc-dns", Name: "dns", Type: "http",
		Labels: map[string]string{"source": "dns", "zone_id": "z1"},
	}); err != nil {
		t.Fatal(err)
	}

	s.pruneStackServices("a1", map[string]bool{"db": true})

	ids := map[string]bool{}
	services, err := s.db.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range services {
		ids[svc.ID] = true
	}
	if ids["stackport-a1-web-8080-tcp"] {
		t.Errorf("vanished stack row kept: %v", ids)
	}
	for _, keep := range []string{"stackport-a1-db-5432-tcp", "stackport-a2-web-8081-tcp", "manual-1", "svc-dns"} {
		if !ids[keep] {
			t.Errorf("row wrongly pruned: %s (%v)", keep, ids)
		}
	}
}

// agent IP 取数：registry 命中优先，离线退回 DB 档案，都没有则不建行
func TestStackAgentIPFallbacks(t *testing.T) {
	// registry 优先（覆盖前两个用例，这里校验顺序：registry 与 DB 冲突取 registry）
	s := covNewServer(t)
	a := covFakeAgent(t, s, "reg", nil, nil)
	a.IP = "10.0.1.1"
	if err := s.db.UpsertAgent(&storage.Agent{ID: "reg", IP: "10.0.9.9"}); err != nil {
		t.Fatal(err)
	}
	if got := s.stackAgentIP("reg"); got != "10.0.1.1" {
		t.Errorf("registry IP = %q", got)
	}

	// DB 档案兜底（registry 无此 agent）
	if err := s.db.UpsertAgent(&storage.Agent{ID: "dbonly", IP: "10.0.7.7"}); err != nil {
		t.Fatal(err)
	}
	if got := s.stackAgentIP("dbonly"); got != "10.0.7.7" {
		t.Errorf("db IP = %q", got)
	}

	// 无处可查 → 空串（不建行）
	if got := s.stackAgentIP("nobody"); got != "" {
		t.Errorf("missing agent IP = %q", got)
	}
	s.cacheStack("nobody", stackItem("web", stackPort(8080, 80, "tcp")))
	if services, err := s.db.ListServices(); err != nil || len(services) != 0 {
		t.Errorf("no-host rows = %+v err=%v", services, err)
	}
}

// 条目结构缺项不 panic（services 缺失/非 map 元素/ports 非切片/端口非 map）
func TestStackServiceSyncMalformedItem(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"
	s.cacheStack("a1", map[string]interface{}{"name": "empty"})
	s.cacheStack("a1", map[string]interface{}{
		"name": "messy",
		"services": []interface{}{
			"not-a-map",
			map[string]interface{}{"name": "no-ports"},
			map[string]interface{}{"name": "bad-ports", "ports": "x"},
			map[string]interface{}{"name": "bad-port", "ports": []interface{}{1, map[string]interface{}{"hostPort": "notnum"}}},
		},
	})
	services, err := s.db.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 0 {
		t.Errorf("malformed item created rows: %+v", services)
	}
}

// db 故障：联动尽力而为，只记日志不 panic（upsert / list 两条错误分支）
func TestStackServiceSyncDBError(t *testing.T) {
	s := covNewServer(t)
	a := covFakeAgent(t, s, "a1", nil, nil)
	a.IP = "10.0.0.5"
	covCloseDB(t, s)
	s.cacheStack("a1", stackItem("web", stackPort(8080, 80, "tcp")))
	s.pruneStackServices("a1", map[string]bool{})
}

// handler 接线：列表成功后对账（第二次列表少一个 stack → 台账行下线）
func TestHandleAgentStacksPrunesInventoryRows(t *testing.T) {
	s := covNewServer(t)
	var stacks []interface{}
	a := covFakeAgent(t, s, "a1", []string{"docker-api"}, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "stack.list" {
			return covOKPayload(stacks)
		}
		return nil
	})
	a.IP = "10.0.0.5"

	covRun := func() *httptest.ResponseRecorder {
		rec := covRec()
		s.handleAgentStacks(rec, covReq(http.MethodGet, "/api/stacks/agents/a1", nil), "a1")
		return rec
	}
	if rec := covRun(); rec.Code != http.StatusOK {
		t.Fatalf("first list = %d, body: %s", rec.Code, rec.Body.String())
	}
	stacks = []interface{}{
		stackItem("web", stackPort(8080, 80, "tcp")),
		stackItem("db", stackPort(5432, 5432, "tcp")),
	}
	if rec := covRun(); rec.Code != http.StatusOK {
		t.Fatalf("two stacks = %d, body: %s", rec.Code, rec.Body.String())
	}
	// 只上报 web：db 台账行应下线
	stacks = []interface{}{stackItem("web", stackPort(8080, 80, "tcp"))}
	if rec := covRun(); rec.Code != http.StatusOK {
		t.Fatalf("shrink = %d, body: %s", rec.Code, rec.Body.String())
	}
	ids := map[string]bool{}
	services, err := s.db.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range services {
		ids[svc.ID] = true
	}
	if !ids["stackport-a1-web-8080-tcp"] || ids["stackport-a1-db-5432-tcp"] {
		t.Errorf("ids = %v", ids)
	}
}
