// Package tencent 实现腾讯云 CVM 的 ComputeProvider 适配器：手写 API 3.0
// 客户端（签名复用 internal/tcsign）直连 cvm.tencentcloudapi.com（POST JSON，
// 地域走 X-TC-Region 头），设计见 docs/design/cloud-provider.md 片 4。与阿里云
// 同口径：SDK 难注入 httptest endpoint，手写客户端换全链路可测性。
// Quota/Billing 子片未落：配额服务契约未经真实账号核对，不硬造（随片 4 验收解锁）。
package tencent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/cloud"
	"github.com/cuihairu/cockpit/internal/tcsign"
)

const (
	apiVersion  = "2017-03-12"
	service     = "cvm"
	defaultBase = "https://cvm.tencentcloudapi.com"
	pageSize    = 100
	maxPages    = 100
)

// Provider 腾讯云 CVM 适配器（cloud.ComputeProvider）。
type Provider struct {
	secretID  string
	secretKey string
	region    string
	base      string
	client    *http.Client
}

// New 创建适配器；secretID/secretKey/region 任一为空返回 nil（未配置语义，
// 接线层据此跳过注册，同 internal/dns 与 aliyun 惯例）。region 是列表查询与
// 创建缺省的地域（CVM API 按 X-TC-Region 头携带地域，无账号级全局列表）。
func New(secretID, secretKey, region string) *Provider {
	if secretID == "" || secretKey == "" || region == "" {
		return nil
	}
	return &Provider{
		secretID:  secretID,
		secretKey: secretKey,
		region:    region,
		base:      defaultBase,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// NewWithBase 测试用：指定 base URL（httptest server）。
func NewWithBase(secretID, secretKey, region, base string) *Provider {
	return &Provider{
		secretID:  secretID,
		secretKey: secretKey,
		region:    region,
		base:      base,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Identity 厂商标识。
func (p *Provider) Identity() cloud.ProviderIdentity {
	return cloud.ProviderIdentity{
		Vendor:      "tencent",
		DisplayName: "腾讯云 CVM",
		Regions:     []string{p.region},
	}
}

// Capabilities 本片只声明 compute；quota/billing 子片落地后再声明（注册表
// fail-closed：声明即校验实现，见 internal/cloud/registry.go）。
func (p *Provider) Capabilities() []cloud.Capability {
	return []cloud.Capability{cloud.CapabilityCompute}
}

// cvmInstance DescribeInstances 条目（仅取适配所需字段）。
type cvmInstance struct {
	InstanceId    string `json:"InstanceId"`
	InstanceName  string `json:"InstanceName"`
	InstanceState string `json:"InstanceState"`
	InstanceType  string `json:"InstanceType"`
	ChargeType    string `json:"ChargeType"`
	Placement     struct {
		Zone string `json:"Zone"`
	} `json:"Placement"`
}

// mapInstance 归一化资源模型。AccountID 留空：账号归属由接线层按
// cloud_accounts 回填，适配器不猜。Region 取查询地域（CVM 条目不携带地域，
// DescribeInstances 是地域级查询，不推断）。状态只映射厂商文档枚举的原值，
// 过渡态（STARTING/STOPPING/REBOOTING/SHUTDOWN）与异常态（LAUNCH_FAILED）
// 归 unknown 且原文保留，不推断。BillingContinues 仅按计费模式的事实填充：
// PREPAID 持续计费至到期、与电源态无关。
func mapInstance(e cvmInstance, region string) cloud.Instance {
	return cloud.Instance{
		NativeID:         e.InstanceId,
		Name:             e.InstanceName,
		Region:           region,
		Zone:             e.Placement.Zone,
		Spec:             e.InstanceType,
		Status:           mapState(e.InstanceState),
		BillingMode:      mapBilling(e.ChargeType),
		BillingContinues: e.ChargeType == "PREPAID",
		RawVendorStatus:  e.InstanceState,
	}
}

// mapState 厂商状态 → 统一枚举；非文档值一律 unknown。
func mapState(s string) cloud.InstanceStatus {
	switch s {
	case "PENDING":
		return cloud.InstanceCreating
	case "RUNNING":
		return cloud.InstanceRunning
	case "STOPPED":
		return cloud.InstanceStopped
	case "TERMINATING":
		return cloud.InstanceDeleting
	default:
		return cloud.InstanceUnknown
	}
}

// mapBilling 计费模式：文档枚举 PREPAID/SPOTPAID/POSTPAID_BY_HOUR，缺省按量。
func mapBilling(charge string) cloud.BillingMode {
	switch charge {
	case "PREPAID":
		return cloud.BillingPrepaid
	case "SPOTPAID":
		return cloud.BillingSpot
	default:
		return cloud.BillingOnDemand
	}
}

// do 构造 TC3 签名、并发 POST JSON；返回已剥壳且无错误的 Response 原文。
// 非 200 与 200 携带 Error 都归一为 error（错误消息只含腾讯云返回的
// Code/Message，绝不拼 secret，D7）。地域由调用方按请求语义给出（列表查
// 询地域、创建可覆盖），随 X-TC-Region 头参与签名。
func (p *Provider) do(ctx context.Context, action, region string, payload []byte) (json.RawMessage, error) {
	// host 参与签名（TC3 签 content-type/host/x-tc-action 三头），取请求实际
	// 发送的 Host：默认端点即 cvm.tencentcloudapi.com，测试端点即 127.0.0.1:port。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	ts := time.Now().Unix()
	auth := tcsign.Authorization(p.secretID, p.secretKey, service, req.URL.Host, action, ts, payload)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", apiVersion)
	req.Header.Set("X-TC-Region", region)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("Authorization", auth)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cvm request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cvm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Response struct {
				Error struct {
					Code    string `json:"Code"`
					Message string `json:"Message"`
				} `json:"Error"`
			} `json:"Response"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Response.Error.Code != "" {
			return nil, fmt.Errorf("cvm error %s: %s", e.Response.Error.Code, e.Response.Error.Message)
		}
		return nil, fmt.Errorf("cvm error: %s", strings.TrimSpace(string(body)))
	}
	// 200 也检查 Response.Error：部分产品把失败包进 200 响应体，fail-closed。
	var env struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("unmarshal cvm response: %w", err)
	}
	var errBox struct {
		Error struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	}
	if err := json.Unmarshal(env.Response, &errBox); err != nil {
		return nil, fmt.Errorf("unmarshal cvm response: %w", err)
	}
	if errBox.Error.Code != "" || errBox.Error.Message != "" {
		return nil, fmt.Errorf("cvm error %s: %s", errBox.Error.Code, errBox.Error.Message)
	}
	return env.Response, nil
}

// ListInstances 分页拉取地域内全部 CVM 实例（页大小 100）。厂商分页异常
// （页非空但 TotalCount 永不满足）按页数上限熔断报错，不静默截断。
func (p *Provider) ListInstances(ctx context.Context) ([]cloud.Instance, error) {
	var out []cloud.Instance
	for page := 1; page <= maxPages; page++ {
		raw, err := p.do(ctx, "DescribeInstances", p.region, mustJSON(map[string]int{
			"Limit": pageSize, "Offset": (page - 1) * pageSize,
		}))
		if err != nil {
			return nil, err
		}
		var resp struct {
			TotalCount  int           `json:"TotalCount"`
			InstanceSet []cvmInstance `json:"InstanceSet"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal DescribeInstances response: %w", err)
		}
		for _, it := range resp.InstanceSet {
			out = append(out, mapInstance(it, p.region))
		}
		if len(resp.InstanceSet) == 0 || len(out) >= resp.TotalCount {
			return out, nil
		}
	}
	return nil, fmt.Errorf("cvm describe instances: pagination exceeded %d pages", maxPages)
}

// getInstance 按 ID 精确拉一台（DescribeInstances 的 InstanceIds 过滤）。
func (p *Provider) getInstance(ctx context.Context, id string) (cvmInstance, bool, error) {
	raw, err := p.do(ctx, "DescribeInstances", p.region, mustJSON(struct {
		InstanceIds []string `json:"InstanceIds"`
	}{InstanceIds: []string{id}}))
	if err != nil {
		return cvmInstance{}, false, err
	}
	var resp struct {
		InstanceSet []cvmInstance `json:"InstanceSet"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return cvmInstance{}, false, fmt.Errorf("unmarshal DescribeInstances response: %w", err)
	}
	if len(resp.InstanceSet) == 0 {
		return cvmInstance{}, false, nil
	}
	return resp.InstanceSet[0], true, nil
}

// CreateInstance 发起 RunInstances（单台）。必需字段缺失在客户端即拒绝，
// 不发厂商请求；竞价走 SPOTPAID、包月走 PREPAID、缺省 POSTPAID_BY_HOUR。
func (p *Provider) CreateInstance(ctx context.Context, spec cloud.InstanceSpec) (cloud.Operation, error) {
	region := spec.Region
	if region == "" {
		region = p.region
	}
	if spec.Spec == "" {
		return cloud.Operation{}, fmt.Errorf("cvm create: instance type (spec) is required")
	}
	if spec.ImageID == "" {
		return cloud.Operation{}, fmt.Errorf("cvm create: image id is required")
	}
	if spec.SecurityGroupID == "" {
		return cloud.Operation{}, fmt.Errorf("cvm create: security group id is required")
	}
	charge := "POSTPAID_BY_HOUR"
	switch spec.BillingMode {
	case cloud.BillingPrepaid:
		charge = "PREPAID"
	case cloud.BillingSpot:
		charge = "SPOTPAID"
	}
	req := struct {
		InstanceType string `json:"InstanceType"`
		ImageId      string `json:"ImageId"`
		Placement    *struct {
			Zone string `json:"Zone,omitempty"`
		} `json:"Placement,omitempty"`
		InstanceChargeType  string `json:"InstanceChargeType"`
		InstanceCount       int    `json:"InstanceCount"`
		VirtualPrivateCloud *struct {
			SecurityGroupIds []string `json:"SecurityGroupIds,omitempty"`
		} `json:"VirtualPrivateCloud,omitempty"`
		InstanceName string `json:"InstanceName,omitempty"`
	}{
		InstanceType:       spec.Spec,
		ImageId:            spec.ImageID,
		InstanceChargeType: charge,
		InstanceCount:      1,
		InstanceName:       spec.Name,
	}
	if spec.Zone != "" {
		req.Placement = &struct {
			Zone string `json:"Zone,omitempty"`
		}{Zone: spec.Zone}
	}
	req.VirtualPrivateCloud = &struct {
		SecurityGroupIds []string `json:"SecurityGroupIds,omitempty"`
	}{SecurityGroupIds: []string{spec.SecurityGroupID}}

	raw, err := p.do(ctx, "RunInstances", region, mustJSON(req))
	if err != nil {
		return cloud.Operation{}, err
	}
	var resp struct {
		RequestId     string   `json:"RequestId"`
		InstanceIdSet []string `json:"InstanceIdSet"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return cloud.Operation{}, fmt.Errorf("unmarshal RunInstances response: %w", err)
	}
	if len(resp.InstanceIdSet) == 0 {
		return cloud.Operation{}, fmt.Errorf("cvm create: response carries no instance id")
	}
	op := cloud.NewOperation(cloud.OperationCreate, resp.RequestId, time.Now().UTC())
	op.NativeID = resp.InstanceIdSet[0]
	return op, nil
}

// StartInstance 异步开机（CVM StartInstances）。
func (p *Provider) StartInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationStart, "StartInstances", id)
}

// StopInstance 异步关机（CVM StopInstances，默认软关机不带强制）。
func (p *Provider) StopInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationStop, "StopInstances", id)
}

// DeleteInstance 异步销毁（CVM TerminateInstances，默认只销毁计费资源）。
func (p *Provider) DeleteInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationDelete, "TerminateInstances", id)
}

// instanceAction 单实例动作：CVM 启停删同构（InstanceIds 数组 → RequestId），
// 错误同步返回（受理即无错），完成态由 GetOperation 轮询实例状态推进。
func (p *Provider) instanceAction(ctx context.Context, kind cloud.OperationKind, action, id string) (cloud.Operation, error) {
	if id == "" {
		return cloud.Operation{}, fmt.Errorf("cvm %s: instance id is required", kind)
	}
	raw, err := p.do(ctx, action, p.region, mustJSON(struct {
		InstanceIds []string `json:"InstanceIds"`
	}{InstanceIds: []string{id}}))
	if err != nil {
		return cloud.Operation{}, err
	}
	var resp struct {
		RequestId string `json:"RequestId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return cloud.Operation{}, fmt.Errorf("unmarshal %s response: %w", action, err)
	}
	op := cloud.NewOperation(kind, resp.RequestId, time.Now().UTC())
	op.NativeID = id
	return op, nil
}

// GetOperation 轮询推进操作状态。CVM 无任务查询 API，以目标实例状态推断：
// create=实例出现即成功；start=RUNNING；stop=STOPPED；delete=查不到即成功。
// 未到完成态输出 running。CVM 的失败同步发生在请求时（do 非 200 或带
// Error 即报错），本面不制造 failed 态。
func (p *Provider) GetOperation(ctx context.Context, op cloud.Operation) (cloud.Operation, error) {
	if op.IsTerminal() {
		return op, nil
	}
	if op.Kind != cloud.OperationCreate && op.Kind != cloud.OperationStart &&
		op.Kind != cloud.OperationStop && op.Kind != cloud.OperationDelete {
		return op, fmt.Errorf("cvm get operation: unknown operation kind %q", op.Kind)
	}
	it, found, err := p.getInstance(ctx, op.NativeID)
	if err != nil {
		return op, err
	}
	var done bool
	switch op.Kind {
	case cloud.OperationCreate:
		done = found
	case cloud.OperationStart:
		done = found && it.InstanceState == "RUNNING"
	case cloud.OperationStop:
		done = found && it.InstanceState == "STOPPED"
	default: // delete（上面已收敛为四类）
		done = !found
	}
	cur := op
	if cur.Status == cloud.OperationPending {
		// pending → running 恒合法（前面已排除终态），错误分支不可达。
		cur, _ = cur.WithStatus(cloud.OperationRunning, time.Now().UTC())
	}
	if !done {
		return cur, nil
	}
	// running → succeeded 恒合法（同上）。
	finished, _ := cur.MarkSucceeded(cur.NativeID, time.Now().UTC())
	return finished, nil
}

// mustJSON 序列化请求体；序列化失败不可能发生（纯 struct/map），防御性报错。
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("cvm: marshal request payload: " + err.Error())
	}
	return b
}
