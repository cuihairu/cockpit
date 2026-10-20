package aliyun

// aliyun_test.go 阿里云 ECS 适配器测试：桩侧经 internal/alisign 复算签名验证
// RPC V1 参数配套 + 全操作契约（列表分页/创建校验/启停删/操作轮询状态机），
// 全程 httptest fake server 零真连（docs/design/cloud-provider.md 片 3 门禁）。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/alisign"
	"github.com/cuihairu/cockpit/internal/cloud"
)

// fakeECS 记录每次请求的 action 与 query；respond 按 action 分发响应。
type fakeECS struct {
	respond func(action string, q url.Values) (int, string)
	actions []string
	queries []url.Values
}

// newFakeECS 起一个复算签名的 mock ECS 端点，返回桩与指向该端点的适配器。
func newFakeECS(t *testing.T, respond func(action string, q url.Values) (int, string)) (*fakeECS, *Provider) {
	t.Helper()
	f := &fakeECS{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.actions = append(f.actions, q.Get("Action"))
		f.queries = append(f.queries, q)
		params := url.Values{}
		for k, v := range q {
			if k != "Signature" {
				params[k] = v
			}
		}
		stringToSign := "GET&%2F&" + alisign.PercentEncode(alisign.CanonicalQuery(params))
		if want := alisign.Sign(stringToSign, "sk-test"); q.Get("Signature") != want {
			t.Errorf("signature mismatch: got %q, want %q", q.Get("Signature"), want)
		}
		code, body := respond(q.Get("Action"), q)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return f, NewWithBase("ak-test", "sk-test", "cn-hangzhou-test", srv.URL)
}

// requireCommon 公共参数断言（签名配套已由桩侧复算覆盖）。
func requireCommon(t *testing.T, action string, q url.Values) {
	t.Helper()
	if q.Get("Action") != action {
		t.Errorf("Action = %q, want %q", q.Get("Action"), action)
	}
	for _, k := range []string{"Format", "Version", "AccessKeyId", "SignatureMethod", "SignatureVersion", "SignatureNonce", "Timestamp"} {
		if q.Get(k) == "" {
			t.Errorf("missing common param %s", k)
		}
	}
	if q.Get("AccessKeyId") != "ak-test" || q.Get("SignatureMethod") != "HMAC-SHA1" ||
		q.Get("Version") != "2014-05-26" || q.Get("Format") != "json" {
		t.Errorf("common param values wrong: %v", q)
	}
}

// instancesBody 组装 DescribeInstances 响应。
func instancesBody(total int, instances ...string) string {
	return fmt.Sprintf(`{"TotalCount":%d,"Instances":{"Instance":[%s]}}`, total, strings.Join(instances, ","))
}

// oneInstance 单实例响应条目（GetOperation 轮询用，状态可配）。
func oneInstance(id, status string) string {
	return fmt.Sprintf(`{"InstanceId":%q,"InstanceName":"x","Status":%q,"InstanceType":"ecs.c7.large","ZoneId":"cn-hangzhou-test-a","RegionId":"cn-hangzhou-test","InstanceChargeType":"PostPaid"}`, id, status)
}

const (
	instRunning        = `{"InstanceId":"i-run","InstanceName":"web-1","Status":"Running","InstanceType":"ecs.c7.large","ZoneId":"cn-hangzhou-test-a","RegionId":"cn-hangzhou-test","InstanceChargeType":"PostPaid"}`
	instPendingPrepaid = `{"InstanceId":"i-pre","InstanceName":"box-2","Status":"Pending","InstanceType":"ecs.g7.xlarge","ZoneId":"cn-hangzhou-test-b","RegionId":"cn-hangzhou-test","InstanceChargeType":"PrePaid","ExpiredTime":"2027-01-01T00:00:00Z"}`
	instStoppingSpot   = `{"InstanceId":"i-spot","InstanceName":"spot-1","Status":"Stopping","InstanceType":"ecs.s7.large","ZoneId":"cn-hangzhou-test-a","RegionId":"cn-hangzhou-test","InstanceChargeType":"PostPaid","SpotStrategy":"SpotWithPrice"}`
	instDeleting       = `{"InstanceId":"i-del","InstanceName":"old","Status":"Deleting","InstanceType":"ecs.t7.small","ZoneId":"cn-hangzhou-test-b","RegionId":"cn-hangzhou-test","InstanceChargeType":"PostPaid"}`
	instStarting       = `{"InstanceId":"i-start","InstanceName":"boot","Status":"Starting","InstanceType":"ecs.r7.large","ZoneId":"cn-hangzhou-test-a","RegionId":"cn-hangzhou-test","InstanceChargeType":"PrePaid"}`
	instStopped        = `{"InstanceId":"i-stop","InstanceName":"idle","Status":"Stopped","InstanceType":"ecs.t7.small","ZoneId":"cn-hangzhou-test-b","RegionId":"cn-hangzhou-test","InstanceChargeType":"PostPaid"}`
)

// fullSpec 完整创建意图（on-demand）。
func fullSpec() cloud.InstanceSpec {
	return cloud.InstanceSpec{
		Name: "box-1", Region: "cn-hangzhou-test", Zone: "cn-hangzhou-test-b",
		Spec: "ecs.c7.large", ImageID: "img-1", SecurityGroupID: "sg-1",
		BillingMode: cloud.BillingOnDemand,
	}
}

// pendingOp 构造一个刚受理（pending）的操作。
func pendingOp(kind cloud.OperationKind, nativeID string) cloud.Operation {
	return cloud.Operation{
		ID: string(kind) + "-r", Kind: kind, NativeID: nativeID,
		VendorRequestID: "r", Status: cloud.OperationPending,
		UpdatedAt: time.Now().UTC(),
	}
}

func TestNewRequiresCredentials(t *testing.T) {
	if New("", "sk", "cn-hangzhou") != nil || New("ak", "", "cn-hangzhou") != nil || New("ak", "sk", "") != nil {
		t.Fatal("missing credential or region should yield nil provider")
	}
	if New("ak", "sk", "cn-hangzhou") == nil {
		t.Fatal("complete credentials should yield provider")
	}
}

func TestIdentityCapabilities(t *testing.T) {
	p := New("ak", "sk", "cn-hangzhou-test")
	id := p.Identity()
	if id.Vendor != "aliyun" || id.DisplayName == "" {
		t.Fatalf("identity = %+v", id)
	}
	if len(id.Regions) != 1 || id.Regions[0] != "cn-hangzhou-test" {
		t.Fatalf("regions = %v", id.Regions)
	}
	if !cloud.Supports(p, cloud.CapabilityCompute) {
		t.Fatal("compute capability must be declared")
	}
	if cloud.Supports(p, cloud.CapabilityQuota) || cloud.Supports(p, cloud.CapabilityBilling) {
		t.Fatal("quota/billing slices not landed yet, must not be declared")
	}
}

// TestListInstancesContract 分页拉全 + 字段归一 + 公共参数/签名契约。
func TestListInstancesContract(t *testing.T) {
	f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		requireCommon(t, "DescribeInstances", q)
		if q.Get("RegionId") != "cn-hangzhou-test" {
			t.Errorf("RegionId = %q", q.Get("RegionId"))
		}
		if q.Get("PageSize") != "100" {
			t.Errorf("PageSize = %q", q.Get("PageSize"))
		}
		switch q.Get("PageNumber") {
		case "1":
			return 200, instancesBody(2, instRunning)
		case "2":
			return 200, instancesBody(2, instPendingPrepaid)
		}
		return 200, instancesBody(2)
	})
	insts, err := p.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(f.actions) != 2 {
		t.Fatalf("expected 2 pages, got %d actions %v", len(f.actions), f.actions)
	}
	if f.actions[1] != "DescribeInstances" || f.queries[1].Get("PageNumber") != "2" {
		t.Fatalf("second page query = %v", f.queries[1])
	}
	if len(insts) != 2 {
		t.Fatalf("instances = %+v", insts)
	}
	got := insts[0]
	if got.NativeID != "i-run" || got.Name != "web-1" || got.Region != "cn-hangzhou-test" ||
		got.Zone != "cn-hangzhou-test-a" || got.Spec != "ecs.c7.large" {
		t.Fatalf("instance fields = %+v", got)
	}
	if got.Status != cloud.InstanceRunning || got.RawVendorStatus != "Running" {
		t.Fatalf("status = %q raw %q", got.Status, got.RawVendorStatus)
	}
	if got.BillingMode != cloud.BillingOnDemand || got.BillingContinues {
		t.Fatalf("on-demand billing = %q continues %v", got.BillingMode, got.BillingContinues)
	}
	if got.AccountID != "" {
		t.Fatalf("adapter must not fabricate AccountID, got %q", got.AccountID)
	}
	pre := insts[1]
	if pre.Status != cloud.InstanceCreating || pre.BillingMode != cloud.BillingPrepaid || !pre.BillingContinues {
		t.Fatalf("prepaid pending = %+v", pre)
	}
}

// TestListInstancesMapping 过渡态归 unknown（原文保留）、竞价识别、删除态。
func TestListInstancesMapping(t *testing.T) {
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, instancesBody(5, instStoppingSpot, instDeleting, instStarting, instPendingPrepaid, instStopped)
	})
	insts, err := p.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	byID := map[string]cloud.Instance{}
	for _, it := range insts {
		byID[it.NativeID] = it
	}
	if len(byID) != 5 {
		t.Fatalf("instances = %+v", insts)
	}
	if stopped := byID["i-stop"]; stopped.Status != cloud.InstanceStopped || stopped.BillingContinues {
		t.Fatalf("stopped on-demand = %+v", stopped)
	}
	spot := byID["i-spot"]
	if spot.Status != cloud.InstanceUnknown || spot.RawVendorStatus != "Stopping" || spot.BillingMode != cloud.BillingSpot {
		t.Fatalf("stopping spot = %+v", spot)
	}
	del := byID["i-del"]
	if del.Status != cloud.InstanceDeleting || del.BillingMode != cloud.BillingOnDemand {
		t.Fatalf("deleting = %+v", del)
	}
	starting := byID["i-start"]
	if starting.Status != cloud.InstanceUnknown || starting.BillingMode != cloud.BillingPrepaid || !starting.BillingContinues {
		t.Fatalf("starting prepaid = %+v", starting)
	}
}

// TestListInstancesEmpty 空地域一次往返即返回。
func TestListInstancesEmpty(t *testing.T) {
	f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, instancesBody(0)
	})
	insts, err := p.ListInstances(context.Background())
	if err != nil || len(insts) != 0 {
		t.Fatalf("ListInstances = %+v, %v", insts, err)
	}
	if len(f.actions) != 1 {
		t.Fatalf("expected single page, got %d", len(f.actions))
	}
}

// TestListInstancesPaginationCap 厂商分页说谎（页非空、TotalCount 永不满足）
// 按页数上限熔断报错，不静默截断。
func TestListInstancesPaginationCap(t *testing.T) {
	f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, instancesBody(1000, instRunning)
	})
	_, err := p.ListInstances(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pagination") {
		t.Fatalf("expected pagination cap error, got %v", err)
	}
	if len(f.actions) != 100 {
		t.Fatalf("expected 100 pages before fuse, got %d", len(f.actions))
	}
}

// TestRequestErrors 非统一错误契约：Code/Message 归一、无 Code 兜底原文、
// 坏 JSON 报 unmarshal；全部五个操作面 + GetOperation 都要走通各自分支。
func TestRequestErrors(t *testing.T) {
	ctx := context.Background()

	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 400, `{"RequestId":"r","Code":"InvalidInstanceId.NotFound","Message":"The specified instance does not exist."}`
	})
	codeErr := "ecs error InvalidInstanceId.NotFound: The specified instance does not exist."
	if _, err := p.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), codeErr) {
		t.Fatalf("list: %v", err)
	}
	if _, err := p.CreateInstance(ctx, fullSpec()); err == nil || !strings.Contains(err.Error(), codeErr) {
		t.Fatalf("create: %v", err)
	}
	for _, call := range []func() (cloud.Operation, error){
		func() (cloud.Operation, error) { return p.StartInstance(ctx, "i-1") },
		func() (cloud.Operation, error) { return p.StopInstance(ctx, "i-1") },
		func() (cloud.Operation, error) { return p.DeleteInstance(ctx, "i-1") },
	} {
		if _, err := call(); err == nil || !strings.Contains(err.Error(), codeErr) {
			t.Fatalf("instance action: %v", err)
		}
	}
	op, err := p.GetOperation(ctx, pendingOp(cloud.OperationStart, "i-1"))
	if err == nil || !strings.Contains(err.Error(), codeErr) {
		t.Fatalf("get operation: %v", err)
	}
	if op.Status != cloud.OperationPending {
		t.Fatalf("poll error must return op unchanged, got %q", op.Status)
	}

	_, raw := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 403, `AccessDenied`
	})
	if _, err := raw.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "ecs error: AccessDenied") {
		t.Fatalf("raw fallback: %v", err)
	}

	_, bad := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, `{`
	})
	if _, err := bad.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal instances") {
		t.Fatalf("list unmarshal: %v", err)
	}
	if _, err := bad.CreateInstance(ctx, fullSpec()); err == nil || !strings.Contains(err.Error(), "unmarshal run instances") {
		t.Fatalf("create unmarshal: %v", err)
	}
	for _, m := range []struct {
		name string
		call func() (cloud.Operation, error)
	}{
		{"StartInstance", func() (cloud.Operation, error) { return bad.StartInstance(ctx, "i-1") }},
		{"StopInstance", func() (cloud.Operation, error) { return bad.StopInstance(ctx, "i-1") }},
		{"DeleteInstance", func() (cloud.Operation, error) { return bad.DeleteInstance(ctx, "i-1") }},
	} {
		if _, err := m.call(); err == nil || !strings.Contains(err.Error(), "unmarshal "+m.name) {
			t.Fatalf("%s unmarshal: %v", m.name, err)
		}
	}
	if _, err := bad.GetOperation(ctx, pendingOp(cloud.OperationCreate, "i-1")); err == nil ||
		!strings.Contains(err.Error(), "unmarshal instances") {
		t.Fatalf("get operation unmarshal: %v", err)
	}
}

// TestCreateInstanceContract RunInstances 参数组装 + 操作构造。
func TestCreateInstanceContract(t *testing.T) {
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		requireCommon(t, "RunInstances", q)
		if q.Get("RegionId") != "cn-hangzhou-test" || q.Get("InstanceType") != "ecs.c7.large" ||
			q.Get("ImageId") != "img-1" || q.Get("SecurityGroupId") != "sg-1" {
			t.Errorf("create params = %v", q)
		}
		if q.Get("InstanceName") != "box-1" || q.Get("ZoneId") != "cn-hangzhou-test-b" {
			t.Errorf("name/zone params = %v", q)
		}
		if q.Get("InstanceChargeType") != "PostPaid" {
			t.Errorf("charge type = %q", q.Get("InstanceChargeType"))
		}
		if _, ok := q["SpotStrategy"]; ok {
			t.Errorf("on-demand must not carry SpotStrategy: %v", q)
		}
		return 200, `{"RequestId":"req-1","InstanceIdSets":{"InstanceId":["i-new"]}}`
	})
	op, err := p.CreateInstance(context.Background(), fullSpec())
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if op.Kind != cloud.OperationCreate || op.Status != cloud.OperationPending ||
		op.VendorRequestID != "req-1" || op.NativeID != "i-new" || op.UpdatedAt.IsZero() {
		t.Fatalf("operation = %+v", op)
	}
}

// TestCreateInstanceBillingModes prepaid/spot/region 回退的参数差异。
func TestCreateInstanceBillingModes(t *testing.T) {
	ctx := context.Background()
	var last url.Values
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		last = q
		return 200, `{"RequestId":"r","InstanceIdSets":{"InstanceId":["i-1"]}}`
	})

	prepaid := fullSpec()
	prepaid.BillingMode = cloud.BillingPrepaid
	if _, err := p.CreateInstance(ctx, prepaid); err != nil {
		t.Fatalf("prepaid: %v", err)
	}
	if last.Get("InstanceChargeType") != "PrePaid" {
		t.Fatalf("prepaid charge = %q", last.Get("InstanceChargeType"))
	}

	spot := fullSpec()
	spot.BillingMode = cloud.BillingSpot
	if _, err := p.CreateInstance(ctx, spot); err != nil {
		t.Fatalf("spot: %v", err)
	}
	if last.Get("InstanceChargeType") != "PostPaid" || last.Get("SpotStrategy") != "SpotAsPriceGo" {
		t.Fatalf("spot params = %v", last)
	}

	noRegion := fullSpec()
	noRegion.Region = ""
	noRegion.Zone = ""
	if _, err := p.CreateInstance(ctx, noRegion); err != nil {
		t.Fatalf("region fallback: %v", err)
	}
	if last.Get("RegionId") != "cn-hangzhou-test" {
		t.Fatalf("fallback region = %q", last.Get("RegionId"))
	}
	if _, ok := last["ZoneId"]; ok {
		t.Fatalf("empty zone must omit ZoneId: %v", last)
	}
}

// TestCreateInstanceValidation 必需字段缺失在客户端即拒绝（不发厂商请求）。
func TestCreateInstanceValidation(t *testing.T) {
	f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, `{}`
	})
	cases := []struct {
		name string
		mut  func(*cloud.InstanceSpec)
		want string
	}{
		{"spec", func(s *cloud.InstanceSpec) { s.Spec = "" }, "instance type"},
		{"image", func(s *cloud.InstanceSpec) { s.ImageID = "" }, "image id"},
		{"security group", func(s *cloud.InstanceSpec) { s.SecurityGroupID = "" }, "security group"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := fullSpec()
			c.mut(&spec)
			_, err := p.CreateInstance(context.Background(), spec)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
	if len(f.actions) != 0 {
		t.Fatalf("validation failures must not reach vendor, got %v", f.actions)
	}
}

// TestCreateInstanceNoInstanceID 响应不带实例 ID：按无凭据报错，不静默成功。
func TestCreateInstanceNoInstanceID(t *testing.T) {
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, `{"RequestId":"req-2"}`
	})
	_, err := p.CreateInstance(context.Background(), fullSpec())
	if err == nil || !strings.Contains(err.Error(), "no instance id") {
		t.Fatalf("want no-instance-id error, got %v", err)
	}
}

// TestInstanceActions 启停删同构：动作名 + InstanceId + 操作构造；空 ID 拒绝。
func TestInstanceActions(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		action string
		call   func(p *Provider) (cloud.Operation, error)
		kind   cloud.OperationKind
	}{
		{"start", "StartInstance", func(p *Provider) (cloud.Operation, error) { return p.StartInstance(ctx, "i-1") }, cloud.OperationStart},
		{"stop", "StopInstance", func(p *Provider) (cloud.Operation, error) { return p.StopInstance(ctx, "i-1") }, cloud.OperationStop},
		{"delete", "DeleteInstance", func(p *Provider) (cloud.Operation, error) { return p.DeleteInstance(ctx, "i-1") }, cloud.OperationDelete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
				requireCommon(t, c.action, q)
				if q.Get("InstanceId") != "i-1" {
					t.Errorf("InstanceId = %q", q.Get("InstanceId"))
				}
				return 200, `{"RequestId":"req-x"}`
			})
			op, err := c.call(p)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if op.Kind != c.kind || op.Status != cloud.OperationPending ||
				op.VendorRequestID != "req-x" || op.NativeID != "i-1" {
				t.Fatalf("operation = %+v", op)
			}
		})
	}

	f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, `{"RequestId":"r"}`
	})
	for _, call := range []func() (cloud.Operation, error){
		func() (cloud.Operation, error) { return p.StartInstance(ctx, "") },
		func() (cloud.Operation, error) { return p.StopInstance(ctx, "") },
		func() (cloud.Operation, error) { return p.DeleteInstance(ctx, "") },
	} {
		if _, err := call(); err == nil || !strings.Contains(err.Error(), "instance id is required") {
			t.Fatalf("empty id must be rejected: %v", err)
		}
	}
	if len(f.actions) != 0 {
		t.Fatalf("empty id must not reach vendor, got %v", f.actions)
	}
}

// TestGetOperation 轮询状态机：以目标实例状态推断完成态。
func TestGetOperation(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		kind     cloud.OperationKind
		instance string // DescribeInstances 响应（空 = 查不到）
		want     cloud.OperationStatus
	}{
		{"create done when instance appears", cloud.OperationCreate, oneInstance("i-1", "Pending"), cloud.OperationSucceeded},
		{"create running while missing", cloud.OperationCreate, "", cloud.OperationRunning},
		{"start done when running", cloud.OperationStart, oneInstance("i-1", "Running"), cloud.OperationSucceeded},
		{"start running while stopped", cloud.OperationStart, oneInstance("i-1", "Stopped"), cloud.OperationRunning},
		{"stop done when stopped", cloud.OperationStop, oneInstance("i-1", "Stopped"), cloud.OperationSucceeded},
		{"stop running while stopping", cloud.OperationStop, oneInstance("i-1", "Stopping"), cloud.OperationRunning},
		{"delete done when gone", cloud.OperationDelete, "", cloud.OperationSucceeded},
		{"delete running while present", cloud.OperationDelete, oneInstance("i-1", "Running"), cloud.OperationRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
				requireCommon(t, "DescribeInstances", q)
				if q.Get("RegionId") != "cn-hangzhou-test" {
					t.Errorf("RegionId = %q", q.Get("RegionId"))
				}
				if c.instance == "" {
					return 200, instancesBody(0)
				}
				return 200, instancesBody(1, c.instance)
			})
			out, err := p.GetOperation(ctx, pendingOp(c.kind, "i-1"))
			if err != nil {
				t.Fatalf("GetOperation: %v", err)
			}
			if out.Status != c.want {
				t.Fatalf("status = %q, want %q", out.Status, c.want)
			}
			if out.NativeID != "i-1" {
				t.Fatalf("native id = %q", out.NativeID)
			}
		})
	}

	t.Run("instance ids param is json array", func(t *testing.T) {
		f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
			return 200, instancesBody(0)
		})
		if _, err := p.GetOperation(ctx, pendingOp(cloud.OperationDelete, "i-1")); err != nil {
			t.Fatal(err)
		}
		if got := f.queries[0].Get("InstanceIds"); got != `["i-1"]` {
			t.Fatalf("InstanceIds = %q", got)
		}
	})

	t.Run("running input advances without pending step", func(t *testing.T) {
		_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
			return 200, instancesBody(1, oneInstance("i-1", "Running"))
		})
		op := pendingOp(cloud.OperationStart, "i-1")
		op.Status = cloud.OperationRunning
		out, err := p.GetOperation(ctx, op)
		if err != nil {
			t.Fatalf("GetOperation: %v", err)
		}
		if out.Status != cloud.OperationSucceeded {
			t.Fatalf("status = %q", out.Status)
		}
	})

	t.Run("terminal op returned unchanged without polling", func(t *testing.T) {
		f, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
			return 200, instancesBody(0)
		})
		op := pendingOp(cloud.OperationStart, "i-1")
		op.Status = cloud.OperationSucceeded
		out, err := p.GetOperation(ctx, op)
		if err != nil {
			t.Fatalf("GetOperation: %v", err)
		}
		if out.Status != cloud.OperationSucceeded || len(f.actions) != 0 {
			t.Fatalf("terminal op = %+v, calls %v", out, f.actions)
		}
	})

	t.Run("unknown kind rejected", func(t *testing.T) {
		_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
			return 200, instancesBody(0)
		})
		if _, err := p.GetOperation(ctx, pendingOp("rescue", "i-1")); err == nil ||
			!strings.Contains(err.Error(), "unknown operation kind") {
			t.Fatalf("want unknown-kind error, got %v", err)
		}
	})
}

// TestRegistryIntegration 注册表闭环：声明的能力与接口实现配套。
func TestRegistryIntegration(t *testing.T) {
	reg := cloud.NewRegistry()
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, instancesBody(1, instRunning)
	})
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get("aliyun")
	if !ok {
		t.Fatal("provider not found after register")
	}
	compute, ok := got.(cloud.ComputeProvider)
	if !ok {
		t.Fatal("registered provider must implement ComputeProvider")
	}
	insts, err := compute.ListInstances(context.Background())
	if err != nil || len(insts) != 1 || insts[0].NativeID != "i-run" {
		t.Fatalf("ListInstances = %+v, %v", insts, err)
	}
}

// TestTransportFailures 传输层三分支：请求构造失败/连接失败/读体失败。
func TestTransportFailures(t *testing.T) {
	ctx := context.Background()

	bad := NewWithBase("ak-test", "sk-test", "cn-hangzhou-test", "http://127.0.0.1\x7f:1")
	if _, err := bad.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("malformed base: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	dead := NewWithBase("ak-test", "sk-test", "cn-hangzhou-test", srv.URL)
	if _, err := dead.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "ecs request") {
		t.Fatalf("dead server: %v", err)
	}

	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer truncated.Close()
	short := NewWithBase("ak-test", "sk-test", "cn-hangzhou-test", truncated.URL)
	if _, err := short.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "read ecs response") {
		t.Fatalf("truncated body: %v", err)
	}
}

// TestContextCancel ctx 已取消：传输层错误上抛。
func TestContextCancel(t *testing.T) {
	_, p := newFakeECS(t, func(action string, q url.Values) (int, string) {
		return 200, instancesBody(0)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.ListInstances(ctx); err == nil {
		t.Fatal("cancelled context must surface")
	}
}
