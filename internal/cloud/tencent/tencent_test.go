package tencent

// tencent_test.go 腾讯云 CVM 适配器测试：桩侧经 internal/tcsign 复算 TC3 签名
// 验证 API 3.0 参数配套（POST JSON + X-TC-* 头）+ 全操作契约（列表分页/创建
// 校验/启停删/操作轮询状态机），全程 httptest fake server 零真连
// （docs/design/cloud-provider.md 片 4 门禁：真 AccessKey 到位前以此代替闭环）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/tcsign"
)

// fakeCVM 记录每次请求的 action、body 与头；respond 按 action 分发响应。
type fakeCVM struct {
	respond func(action string, body []byte) (int, string)
	actions []string
	bodies  [][]byte
	headers []http.Header
}

// newFakeCVM 起一个复算 TC3 签名的 mock CVM 端点，返回桩与指向该端点的适配器。
func newFakeCVM(t *testing.T, respond func(action string, body []byte) (int, string)) (*fakeCVM, *Provider) {
	t.Helper()
	f := &fakeCVM{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		f.actions = append(f.actions, r.Header.Get("X-TC-Action"))
		f.bodies = append(f.bodies, body)
		f.headers = append(f.headers, r.Header.Clone())
		ts, _ := strconv.ParseInt(r.Header.Get("X-TC-Timestamp"), 10, 64)
		if want := tcsign.Authorization("akid-test", "sk-test", "cvm", r.Host, r.Header.Get("X-TC-Action"), ts, body); r.Header.Get("Authorization") != want {
			t.Errorf("signature mismatch:\n got %q\nwant %q", r.Header.Get("Authorization"), want)
		}
		code, respBody := respond(r.Header.Get("X-TC-Action"), body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return f, NewWithBase("akid-test", "sk-test", "ap-guangzhou-test", srv.URL)
}

// requireCommon 公共头断言（TC3 签名配套已由桩侧复算覆盖；content-type 参与
// canonical request，值必须精确）。
func requireCommon(t *testing.T, action string, h http.Header) {
	t.Helper()
	if h.Get("X-TC-Action") != action {
		t.Errorf("X-TC-Action = %q, want %q", h.Get("X-TC-Action"), action)
	}
	if h.Get("X-TC-Version") != apiVersion {
		t.Errorf("X-TC-Version = %q", h.Get("X-TC-Version"))
	}
	if h.Get("X-TC-Region") != "ap-guangzhou-test" {
		t.Errorf("X-TC-Region = %q", h.Get("X-TC-Region"))
	}
	if h.Get("X-TC-Timestamp") == "" {
		t.Errorf("missing X-TC-Timestamp")
	}
	if h.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", h.Get("Content-Type"))
	}
	if h.Get("Authorization") == "" {
		t.Errorf("missing Authorization")
	}
}

// bodyMap 解析请求体为对象（桩请求永远是 JSON object）。
func bodyMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("request body not json object: %v: %s", err, body)
	}
	return v
}

// cvmBody 组装 CVM 响应外壳。
func cvmBody(inner string) string {
	return fmt.Sprintf(`{"Response":%s}`, inner)
}

// instancesBody 组装 DescribeInstances 响应。
func instancesBody(total int, instances ...string) string {
	return cvmBody(fmt.Sprintf(`{"RequestId":"req-list","TotalCount":%d,"InstanceSet":[%s]}`, total, strings.Join(instances, ",")))
}

// oneInstance 单实例响应条目（GetOperation 轮询用，状态/计费可配）。
func oneInstance(id, state, charge string) string {
	return fmt.Sprintf(`{"InstanceId":%q,"InstanceName":"x","InstanceState":%q,"InstanceType":"cvm.c7.large","ChargeType":%q,"Placement":{"Zone":"ap-guangzhou-test-b"}}`, id, state, charge)
}

const (
	instRunning        = `{"InstanceId":"i-run","InstanceName":"web-1","InstanceState":"RUNNING","InstanceType":"cvm.c7.large","ChargeType":"POSTPAID_BY_HOUR","Placement":{"Zone":"ap-guangzhou-test-a"}}`
	instPendingPrepaid = `{"InstanceId":"i-pre","InstanceName":"box-2","InstanceState":"PENDING","InstanceType":"cvm.g7.xlarge","ChargeType":"PREPAID","Placement":{"Zone":"ap-guangzhou-test-b"}}`
	instTermSpot       = `{"InstanceId":"i-spot","InstanceName":"spot-1","InstanceState":"TERMINATING","InstanceType":"cvm.s7.large","ChargeType":"SPOTPAID","Placement":{"Zone":"ap-guangzhou-test-a"}}`
	instStarting       = `{"InstanceId":"i-start","InstanceName":"boot","InstanceState":"STARTING","InstanceType":"cvm.r7.large","ChargeType":"PREPAID","Placement":{"Zone":"ap-guangzhou-test-a"}}`
	instStopped        = `{"InstanceId":"i-stop","InstanceName":"idle","InstanceState":"STOPPED","InstanceType":"cvm.t7.small","ChargeType":"POSTPAID_BY_HOUR","Placement":{"Zone":"ap-guangzhou-test-b"}}`
	instLaunchFailed   = `{"InstanceId":"i-fail","InstanceName":"bad","InstanceState":"LAUNCH_FAILED","InstanceType":"cvm.t7.small","ChargeType":"POSTPAID_BY_HOUR","Placement":{"Zone":"ap-guangzhou-test-a"}}`
)

// fullSpec 完整创建意图（on-demand）。
func fullSpec() cloud.InstanceSpec {
	return cloud.InstanceSpec{
		Name: "box-1", Region: "ap-guangzhou-test", Zone: "ap-guangzhou-test-b",
		Spec: "cvm.c7.large", ImageID: "img-1", SecurityGroupID: "sg-1",
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
	if New("", "sk", "ap-guangzhou") != nil || New("akid", "", "ap-guangzhou") != nil || New("akid", "sk", "") != nil {
		t.Fatal("missing credential or region should yield nil provider")
	}
	if New("akid", "sk", "ap-guangzhou") == nil {
		t.Fatal("complete credentials should yield provider")
	}
}

func TestIdentityCapabilities(t *testing.T) {
	p := New("akid", "sk", "ap-guangzhou-test")
	id := p.Identity()
	if id.Vendor != "tencent" || id.DisplayName == "" {
		t.Fatalf("identity = %+v", id)
	}
	if len(id.Regions) != 1 || id.Regions[0] != "ap-guangzhou-test" {
		t.Fatalf("regions = %v", id.Regions)
	}
	if !cloud.Supports(p, cloud.CapabilityCompute) {
		t.Fatal("compute capability must be declared")
	}
	if cloud.Supports(p, cloud.CapabilityQuota) || cloud.Supports(p, cloud.CapabilityBilling) {
		t.Fatal("quota/billing slices not landed yet, must not be declared")
	}
}

// TestListInstancesContract 分页拉全 + 字段归一 + 公共头/签名契约。
func TestListInstancesContract(t *testing.T) {
	var f *fakeCVM
	var p *Provider
	f, p = newFakeCVM(t, func(action string, body []byte) (int, string) {
		requireCommon(t, "DescribeInstances", f.headers[len(f.headers)-1])
		b := bodyMap(t, body)
		switch b["Offset"] {
		case float64(0):
			if b["Limit"] != float64(pageSize) {
				t.Errorf("page 1 Limit = %v", b["Limit"])
			}
			return 200, instancesBody(2, instRunning)
		case float64(pageSize):
			return 200, instancesBody(2, instPendingPrepaid)
		}
		t.Errorf("unexpected offset %v", b["Offset"])
		return 200, instancesBody(2)
	})
	insts, err := p.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(f.actions) != 2 || f.actions[1] != "DescribeInstances" {
		t.Fatalf("expected 2 pages, got %d actions %v", len(f.actions), f.actions)
	}
	if len(insts) != 2 {
		t.Fatalf("instances = %+v", insts)
	}
	got := insts[0]
	if got.NativeID != "i-run" || got.Name != "web-1" || got.Region != "ap-guangzhou-test" ||
		got.Zone != "ap-guangzhou-test-a" || got.Spec != "cvm.c7.large" {
		t.Fatalf("instance fields = %+v", got)
	}
	if got.Status != cloud.InstanceRunning || got.RawVendorStatus != "RUNNING" {
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

// TestListInstancesMapping 过渡态归 unknown（原文保留）、竞价识别、删除态、
// 异常态（LAUNCH_FAILED）不映射成任何业务态。
func TestListInstancesMapping(t *testing.T) {
	_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, instancesBody(6, instTermSpot, instStarting, instStopped, instLaunchFailed, instRunning, instPendingPrepaid)
	})
	insts, err := p.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	byID := map[string]cloud.Instance{}
	for _, it := range insts {
		byID[it.NativeID] = it
	}
	if len(byID) != 6 {
		t.Fatalf("instances = %+v", insts)
	}
	if stopped := byID["i-stop"]; stopped.Status != cloud.InstanceStopped || stopped.BillingContinues {
		t.Fatalf("stopped on-demand = %+v", stopped)
	}
	spot := byID["i-spot"]
	if spot.Status != cloud.InstanceDeleting || spot.RawVendorStatus != "TERMINATING" || spot.BillingMode != cloud.BillingSpot {
		t.Fatalf("terminating spot = %+v", spot)
	}
	starting := byID["i-start"]
	if starting.Status != cloud.InstanceUnknown || starting.BillingMode != cloud.BillingPrepaid || !starting.BillingContinues {
		t.Fatalf("starting prepaid = %+v", starting)
	}
	if failed := byID["i-fail"]; failed.Status != cloud.InstanceUnknown || failed.RawVendorStatus != "LAUNCH_FAILED" {
		t.Fatalf("launch failed = %+v", failed)
	}
}

// TestListInstancesEmpty 空地域一次往返即返回。
func TestListInstancesEmpty(t *testing.T) {
	f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
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
	f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, instancesBody(1000, instRunning)
	})
	_, err := p.ListInstances(context.Background())
	if err == nil || !strings.Contains(err.Error(), "pagination") {
		t.Fatalf("expected pagination cap error, got %v", err)
	}
	if len(f.actions) != maxPages {
		t.Fatalf("expected %d pages before fuse, got %d", maxPages, len(f.actions))
	}
	last := bodyMap(t, f.bodies[len(f.bodies)-1])
	if last["Offset"] != float64((maxPages-1)*pageSize) {
		t.Fatalf("last offset = %v", last["Offset"])
	}
}

// TestRequestErrors 错误契约：非 200 带 Error 归一 Code/Message、非 200 无
// Error 兜底原文、200 带 Error 也报错（fail-closed）、坏壳报 unmarshal、
// 坏载荷报动作级 unmarshal；全部五个操作面 + GetOperation 各自走通。
func TestRequestErrors(t *testing.T) {
	ctx := context.Background()

	_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 400, cvmBody(`{"RequestId":"r","Error":{"Code":"InvalidInstanceId.NotFound","Message":"The specified instance does not exist."}}`)
	})
	codeErr := "cvm error InvalidInstanceId.NotFound: The specified instance does not exist."
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

	_, raw := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 403, `AccessDenied`
	})
	if _, err := raw.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "cvm error: AccessDenied") {
		t.Fatalf("raw fallback: %v", err)
	}

	// 200 但 Response 带 Error：部分产品把失败包进成功体，必须报错。
	_, wrapped := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"RequestId":"r","Error":{"Code":"InternalError","Message":"boom"}}`)
	})
	if _, err := wrapped.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "cvm error InternalError: boom") {
		t.Fatalf("200-with-error: %v", err)
	}

	_, badShell := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, `{`
	})
	if _, err := badShell.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal cvm response") {
		t.Fatalf("bad envelope: %v", err)
	}

	// Response 不是对象：外壳可解构但 Error 探测失败 → 同一 unmarshal 错误。
	_, badErrBox := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, `{"Response":123}`
	})
	if _, err := badErrBox.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal cvm response") {
		t.Fatalf("bad error box: %v", err)
	}

	// 各动作载荷级类型不匹配 → 动作名可定位。
	_, mismatchList := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"TotalCount":"oops"}`)
	})
	if _, err := mismatchList.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal DescribeInstances response") {
		t.Fatalf("list unmarshal: %v", err)
	}
	_, mismatchCreate := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"InstanceIdSet":"oops"}`)
	})
	if _, err := mismatchCreate.CreateInstance(ctx, fullSpec()); err == nil || !strings.Contains(err.Error(), "unmarshal RunInstances response") {
		t.Fatalf("create unmarshal: %v", err)
	}
	_, mismatchAction := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"RequestId":123}`)
	})
	if _, err := mismatchAction.StartInstance(ctx, "i-1"); err == nil || !strings.Contains(err.Error(), "unmarshal StartInstances response") {
		t.Fatalf("action unmarshal: %v", err)
	}
}

// TestCreateInstanceContract RunInstances 参数组装 + 操作构造。
func TestCreateInstanceContract(t *testing.T) {
	var f *fakeCVM
	var p *Provider
	f, p = newFakeCVM(t, func(action string, body []byte) (int, string) {
		requireCommon(t, "RunInstances", f.headers[len(f.headers)-1])
		b := bodyMap(t, body)
		if b["InstanceType"] != "cvm.c7.large" || b["ImageId"] != "img-1" ||
			b["InstanceChargeType"] != "POSTPAID_BY_HOUR" || b["InstanceCount"] != float64(1) {
			t.Errorf("create params = %v", b)
		}
		if b["InstanceName"] != "box-1" {
			t.Errorf("name = %v", b["InstanceName"])
		}
		placement, ok := b["Placement"].(map[string]interface{})
		if !ok || placement["Zone"] != "ap-guangzhou-test-b" {
			t.Errorf("placement = %v", b["Placement"])
		}
		vpc, ok := b["VirtualPrivateCloud"].(map[string]interface{})
		if !ok {
			t.Fatalf("missing VirtualPrivateCloud: %v", b)
		}
		sgs, ok := vpc["SecurityGroupIds"].([]interface{})
		if !ok || len(sgs) != 1 || sgs[0] != "sg-1" {
			t.Errorf("security groups = %v", vpc["SecurityGroupIds"])
		}
		return 200, cvmBody(`{"RequestId":"req-1","InstanceIdSet":["i-new"]}`)
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

// TestCreateInstanceBillingModes prepaid/spot/region 回退/空 zone 省略的参数差异。
func TestCreateInstanceBillingModes(t *testing.T) {
	ctx := context.Background()
	var lastBody []byte
	var lastHeader http.Header
	var f *fakeCVM
	var p *Provider
	f, p = newFakeCVM(t, func(action string, body []byte) (int, string) {
		lastBody = body
		lastHeader = f.headers[len(f.headers)-1]
		return 200, cvmBody(`{"RequestId":"r","InstanceIdSet":["i-1"]}`)
	})

	prepaid := fullSpec()
	prepaid.BillingMode = cloud.BillingPrepaid
	if _, err := p.CreateInstance(ctx, prepaid); err != nil {
		t.Fatalf("prepaid: %v", err)
	}
	if b := bodyMap(t, lastBody); b["InstanceChargeType"] != "PREPAID" {
		t.Fatalf("prepaid charge = %v", b["InstanceChargeType"])
	}

	spot := fullSpec()
	spot.BillingMode = cloud.BillingSpot
	if _, err := p.CreateInstance(ctx, spot); err != nil {
		t.Fatalf("spot: %v", err)
	}
	if b := bodyMap(t, lastBody); b["InstanceChargeType"] != "SPOTPAID" {
		t.Fatalf("spot charge = %v", b["InstanceChargeType"])
	}

	noRegion := fullSpec()
	noRegion.Region = ""
	noRegion.Zone = ""
	if _, err := p.CreateInstance(ctx, noRegion); err != nil {
		t.Fatalf("region fallback: %v", err)
	}
	if lastHeader.Get("X-TC-Region") != "ap-guangzhou-test" {
		t.Fatalf("fallback region = %q", lastHeader.Get("X-TC-Region"))
	}
	if _, ok := bodyMap(t, lastBody)["Placement"]; ok {
		t.Fatalf("empty zone must omit Placement: %s", lastBody)
	}
}

// TestCreateInstanceValidation 必需字段缺失在客户端即拒绝（不发厂商请求）。
func TestCreateInstanceValidation(t *testing.T) {
	f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
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

// TestCreateInstanceNoInstanceID 响应不带实例 ID：按错误报出，不静默成功。
func TestCreateInstanceNoInstanceID(t *testing.T) {
	_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"RequestId":"req-2"}`)
	})
	_, err := p.CreateInstance(context.Background(), fullSpec())
	if err == nil || !strings.Contains(err.Error(), "no instance id") {
		t.Fatalf("want no-instance-id error, got %v", err)
	}
}

// TestInstanceActions 启停删同构：动作名 + InstanceIds 数组 + 操作构造；空 ID 拒绝。
func TestInstanceActions(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		action string
		call   func(p *Provider) (cloud.Operation, error)
		kind   cloud.OperationKind
	}{
		{"start", "StartInstances", func(p *Provider) (cloud.Operation, error) { return p.StartInstance(ctx, "i-1") }, cloud.OperationStart},
		{"stop", "StopInstances", func(p *Provider) (cloud.Operation, error) { return p.StopInstance(ctx, "i-1") }, cloud.OperationStop},
		{"delete", "TerminateInstances", func(p *Provider) (cloud.Operation, error) { return p.DeleteInstance(ctx, "i-1") }, cloud.OperationDelete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var f *fakeCVM
			var p *Provider
			f, p = newFakeCVM(t, func(action string, body []byte) (int, string) {
				requireCommon(t, c.action, f.headers[len(f.headers)-1])
				b := bodyMap(t, body)
				ids, ok := b["InstanceIds"].([]interface{})
				if !ok || len(ids) != 1 || ids[0] != "i-1" {
					t.Errorf("InstanceIds = %v", b["InstanceIds"])
				}
				return 200, cvmBody(`{"RequestId":"req-x"}`)
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

	f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, cvmBody(`{"RequestId":"r"}`)
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
		instance string // DescribeInstances 响应条目（空 = 查不到）
		want     cloud.OperationStatus
	}{
		{"create done when instance appears", cloud.OperationCreate, oneInstance("i-1", "PENDING", "POSTPAID_BY_HOUR"), cloud.OperationSucceeded},
		{"create running while missing", cloud.OperationCreate, "", cloud.OperationRunning},
		{"start done when running", cloud.OperationStart, oneInstance("i-1", "RUNNING", "POSTPAID_BY_HOUR"), cloud.OperationSucceeded},
		{"start running while stopped", cloud.OperationStart, oneInstance("i-1", "STOPPED", "POSTPAID_BY_HOUR"), cloud.OperationRunning},
		{"stop done when stopped", cloud.OperationStop, oneInstance("i-1", "STOPPED", "POSTPAID_BY_HOUR"), cloud.OperationSucceeded},
		{"stop running while rebooting", cloud.OperationStop, oneInstance("i-1", "REBOOTING", "POSTPAID_BY_HOUR"), cloud.OperationRunning},
		{"delete done when gone", cloud.OperationDelete, "", cloud.OperationSucceeded},
		{"delete running while present", cloud.OperationDelete, oneInstance("i-1", "RUNNING", "POSTPAID_BY_HOUR"), cloud.OperationRunning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var f *fakeCVM
			var p *Provider
			f, p = newFakeCVM(t, func(action string, body []byte) (int, string) {
				requireCommon(t, "DescribeInstances", f.headers[len(f.headers)-1])
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
		f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
			return 200, instancesBody(0)
		})
		if _, err := p.GetOperation(ctx, pendingOp(cloud.OperationDelete, "i-1")); err != nil {
			t.Fatal(err)
		}
		b := bodyMap(t, f.bodies[0])
		ids, ok := b["InstanceIds"].([]interface{})
		if !ok || len(ids) != 1 || ids[0] != "i-1" {
			t.Fatalf("InstanceIds = %v", b["InstanceIds"])
		}
	})

	t.Run("running input advances without pending step", func(t *testing.T) {
		_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
			return 200, instancesBody(1, oneInstance("i-1", "RUNNING", "POSTPAID_BY_HOUR"))
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
		f, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
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
		_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
			return 200, instancesBody(0)
		})
		if _, err := p.GetOperation(ctx, pendingOp("rescue", "i-1")); err == nil ||
			!strings.Contains(err.Error(), "unknown operation kind") {
			t.Fatalf("want unknown-kind error, got %v", err)
		}
	})

	t.Run("describe response bad payload", func(t *testing.T) {
		_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
			return 200, cvmBody(`{"InstanceSet":"oops"}`)
		})
		if _, err := p.GetOperation(ctx, pendingOp(cloud.OperationStart, "i-1")); err == nil ||
			!strings.Contains(err.Error(), "unmarshal DescribeInstances response") {
			t.Fatalf("get operation unmarshal: %v", err)
		}
	})
}

// TestMustJSONPanics 纯 struct/map 载荷不可能序列化失败；不可序列化类型
// 属防御分支，直接断言 panic（不靠登记豁免）。
func TestMustJSONPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("mustJSON must panic on unmarshalable payload")
		}
	}()
	mustJSON(make(chan int))
}

// TestRegistryIntegration 注册表闭环：声明的能力与接口实现配套。
func TestRegistryIntegration(t *testing.T) {
	reg := cloud.NewRegistry()
	_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, instancesBody(1, instRunning)
	})
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get("tencent")
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

	bad := NewWithBase("akid-test", "sk-test", "ap-guangzhou-test", "http://127.0.0.1\x7f:1")
	if _, err := bad.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("malformed base: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	dead := NewWithBase("akid-test", "sk-test", "ap-guangzhou-test", srv.URL)
	if _, err := dead.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "cvm request") {
		t.Fatalf("dead server: %v", err)
	}

	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer truncated.Close()
	short := NewWithBase("akid-test", "sk-test", "ap-guangzhou-test", truncated.URL)
	if _, err := short.ListInstances(ctx); err == nil || !strings.Contains(err.Error(), "read cvm response") {
		t.Fatalf("truncated body: %v", err)
	}
}

// TestContextCancel ctx 已取消：传输层错误上抛。
func TestContextCancel(t *testing.T) {
	_, p := newFakeCVM(t, func(action string, body []byte) (int, string) {
		return 200, instancesBody(0)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.ListInstances(ctx); err == nil {
		t.Fatal("cancelled context must surface")
	}
}
