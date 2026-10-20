// Package aliyun 实现阿里云 ECS 的 ComputeProvider 适配器：手写 RPC V1 客户端
// （签名复用 internal/alisign）直连 ecs.aliyuncs.com（GET query），设计见
// docs/design/cloud-provider.md 片 3。SDK（tea 栈）难注入 httptest endpoint，
// 手写客户端换全链路可测性（同 internal/dns 惯例）。
// Quota/Billing 子片未落：配额服务契约未经真实账号核对，不硬造（随片 4 验收解锁）。
package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/alisign"
	"github.com/cuihairu/cockpit/internal/cloud"
)

const (
	apiVersion  = "2014-05-26"
	defaultBase = "https://ecs.aliyuncs.com"
	pageSize    = 100
	maxPages    = 100
)

// Provider 阿里云 ECS 适配器（cloud.ComputeProvider）。
type Provider struct {
	accessKey string
	secretKey string
	region    string
	base      string
	client    *http.Client
}

// New 创建适配器；accessKey/secretKey/region 任一为空返回 nil（未配置语义，
// 接线层据此跳过注册，同 internal/dns 惯例）。region 是列表查询与创建缺省的
// 地域（ECS API 按请求携带 RegionId，无账号级全局列表）。
func New(accessKey, secretKey, region string) *Provider {
	if accessKey == "" || secretKey == "" || region == "" {
		return nil
	}
	return &Provider{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		base:      defaultBase,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// NewWithBase 测试用：指定 base URL（httptest server）。
func NewWithBase(accessKey, secretKey, region, base string) *Provider {
	return &Provider{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		base:      base,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Identity 厂商标识。
func (p *Provider) Identity() cloud.ProviderIdentity {
	return cloud.ProviderIdentity{
		Vendor:      "aliyun",
		DisplayName: "阿里云 ECS",
		Regions:     []string{p.region},
	}
}

// Capabilities 本片只声明 compute；quota/billing 子片落地后再声明（注册表
// fail-closed：声明即校验实现，见 internal/cloud/registry.go）。
func (p *Provider) Capabilities() []cloud.Capability {
	return []cloud.Capability{cloud.CapabilityCompute}
}

// ecsInstance DescribeInstances 条目（仅取适配所需字段）。
type ecsInstance struct {
	InstanceId         string `json:"InstanceId"`
	InstanceName       string `json:"InstanceName"`
	Status             string `json:"Status"`
	InstanceType       string `json:"InstanceType"`
	ZoneId             string `json:"ZoneId"`
	RegionId           string `json:"RegionId"`
	InstanceChargeType string `json:"InstanceChargeType"`
	SpotStrategy       string `json:"SpotStrategy"`
}

// instancesResponse DescribeInstances 响应（列表与按 ID 精查共用）。
type instancesResponse struct {
	TotalCount int `json:"TotalCount"`
	Instances  struct {
		Instance []ecsInstance `json:"Instance"`
	} `json:"Instances"`
}

// mapInstance 归一化资源模型。AccountID 留空：账号归属由接线层按
// cloud_accounts 回填，适配器不猜。状态只映射厂商文档枚举的原值，
// 过渡态（Starting/Stopping/Rebooting）归 unknown 且原文保留，不推断。
// BillingContinues 仅按计费模式的事实填充：包月持续计费至到期、与电源态
// 无关；按量实例停机后计算费停收（磁盘另计），不推断为持续。
func mapInstance(e ecsInstance) cloud.Instance {
	return cloud.Instance{
		NativeID:         e.InstanceId,
		Name:             e.InstanceName,
		Region:           e.RegionId,
		Zone:             e.ZoneId,
		Spec:             e.InstanceType,
		Status:           mapStatus(e.Status),
		BillingMode:      mapBilling(e.InstanceChargeType, e.SpotStrategy),
		BillingContinues: e.InstanceChargeType == "PrePaid",
		RawVendorStatus:  e.Status,
	}
}

// mapStatus 厂商状态 → 统一枚举；非文档值一律 unknown。
func mapStatus(s string) cloud.InstanceStatus {
	switch s {
	case "Pending":
		return cloud.InstanceCreating
	case "Running":
		return cloud.InstanceRunning
	case "Stopped":
		return cloud.InstanceStopped
	case "Deleting":
		return cloud.InstanceDeleting
	default:
		return cloud.InstanceUnknown
	}
}

// mapBilling 计费模式：SpotStrategy 携带竞价证据时优先归竞价，其后按
// InstanceChargeType（文档枚举仅 PrePaid/PostPaid，缺省按量）。
func mapBilling(charge, spotStrategy string) cloud.BillingMode {
	switch {
	case spotStrategy == "SpotWithPrice" || spotStrategy == "SpotAsPriceGo":
		return cloud.BillingSpot
	case charge == "PrePaid":
		return cloud.BillingPrepaid
	default:
		return cloud.BillingOnDemand
	}
}

// do 构造公共参数、签名并发 GET；业务数据由调用方解。非 200 都归一为
// error（错误消息只含阿里云返回的 Code/Message，绝不拼 AccessKey/secret，D7）。
func (p *Provider) do(ctx context.Context, action string, params url.Values) ([]byte, error) {
	params.Set("Action", action)
	params.Set("Format", "json")
	params.Set("Version", apiVersion)
	params.Set("AccessKeyId", p.accessKey)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", alisign.Nonce())
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/?"+alisign.SignQuery(p.secretKey, params), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ecs request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ecs response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// RPC 错误响应：{"RequestId","Code","Message",...}
		var e struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(body))
		}
		if e.Code != "" {
			return nil, fmt.Errorf("ecs error %s: %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("ecs error: %s", e.Message)
	}
	return body, nil
}

// ListInstances 分页拉取地域内全部 ECS 实例（页大小 100）。厂商分页异常
// （页非空但 TotalCount 永不满足）按页数上限熔断报错，不静默截断。
func (p *Provider) ListInstances(ctx context.Context) ([]cloud.Instance, error) {
	var out []cloud.Instance
	for page := 1; page <= maxPages; page++ {
		body, err := p.do(ctx, "DescribeInstances", url.Values{
			"RegionId":   {p.region},
			"PageSize":   {strconv.Itoa(pageSize)},
			"PageNumber": {strconv.Itoa(page)},
		})
		if err != nil {
			return nil, err
		}
		var resp instancesResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal instances: %w", err)
		}
		for _, it := range resp.Instances.Instance {
			out = append(out, mapInstance(it))
		}
		if len(resp.Instances.Instance) == 0 || len(out) >= resp.TotalCount {
			return out, nil
		}
	}
	return nil, fmt.Errorf("ecs describe instances: pagination exceeded %d pages", maxPages)
}

// getInstance 按 ID 精确拉一台（DescribeInstances 的 InstanceIds JSON 数组）。
func (p *Provider) getInstance(ctx context.Context, id string) (ecsInstance, bool, error) {
	ids, _ := json.Marshal([]string{id})
	body, err := p.do(ctx, "DescribeInstances", url.Values{
		"RegionId":    {p.region},
		"InstanceIds": {string(ids)},
	})
	if err != nil {
		return ecsInstance{}, false, err
	}
	var resp instancesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ecsInstance{}, false, fmt.Errorf("unmarshal instances: %w", err)
	}
	if len(resp.Instances.Instance) == 0 {
		return ecsInstance{}, false, nil
	}
	return resp.Instances.Instance[0], true, nil
}

// CreateInstance 发起 RunInstances（单台）。必需字段缺失在客户端即拒绝，
// 不发厂商请求；竞价走 SpotAsPriceGo（系统出价，无需限价参数）。
func (p *Provider) CreateInstance(ctx context.Context, spec cloud.InstanceSpec) (cloud.Operation, error) {
	region := spec.Region
	if region == "" {
		region = p.region
	}
	if spec.Spec == "" {
		return cloud.Operation{}, fmt.Errorf("ecs create: instance type (spec) is required")
	}
	if spec.ImageID == "" {
		return cloud.Operation{}, fmt.Errorf("ecs create: image id is required")
	}
	if spec.SecurityGroupID == "" {
		return cloud.Operation{}, fmt.Errorf("ecs create: security group id is required")
	}
	params := url.Values{
		"RegionId":           {region},
		"InstanceType":       {spec.Spec},
		"ImageId":            {spec.ImageID},
		"SecurityGroupId":    {spec.SecurityGroupID},
		"InstanceChargeType": {"PostPaid"},
	}
	if spec.BillingMode == cloud.BillingPrepaid {
		params.Set("InstanceChargeType", "PrePaid")
	}
	if spec.BillingMode == cloud.BillingSpot {
		params.Set("SpotStrategy", "SpotAsPriceGo")
	}
	if spec.Zone != "" {
		params.Set("ZoneId", spec.Zone)
	}
	if spec.Name != "" {
		params.Set("InstanceName", spec.Name)
	}
	body, err := p.do(ctx, "RunInstances", params)
	if err != nil {
		return cloud.Operation{}, err
	}
	var resp struct {
		RequestId      string `json:"RequestId"`
		InstanceIdSets struct {
			InstanceId []string `json:"InstanceId"`
		} `json:"InstanceIdSets"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return cloud.Operation{}, fmt.Errorf("unmarshal run instances: %w", err)
	}
	if len(resp.InstanceIdSets.InstanceId) == 0 {
		return cloud.Operation{}, fmt.Errorf("ecs create: response carries no instance id")
	}
	op := cloud.NewOperation(cloud.OperationCreate, resp.RequestId, time.Now().UTC())
	op.NativeID = resp.InstanceIdSets.InstanceId[0]
	return op, nil
}

// StartInstance 异步开机（ECS StartInstance）。
func (p *Provider) StartInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationStart, "StartInstance", id)
}

// StopInstance 异步关机（ECS StopInstance，默认安全关机不带 ForceStop）。
func (p *Provider) StopInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationStop, "StopInstance", id)
}

// DeleteInstance 异步释放（ECS DeleteInstance，不带 Force）。
func (p *Provider) DeleteInstance(ctx context.Context, id string) (cloud.Operation, error) {
	return p.instanceAction(ctx, cloud.OperationDelete, "DeleteInstance", id)
}

// instanceAction 单实例动作：ECS 启停删同构（InstanceId → RequestId），
// 错误同步返回（受理即无错），完成态由 GetOperation 轮询实例状态推进。
func (p *Provider) instanceAction(ctx context.Context, kind cloud.OperationKind, action, id string) (cloud.Operation, error) {
	if id == "" {
		return cloud.Operation{}, fmt.Errorf("ecs %s: instance id is required", kind)
	}
	body, err := p.do(ctx, action, url.Values{"InstanceId": {id}})
	if err != nil {
		return cloud.Operation{}, err
	}
	var resp struct {
		RequestId string `json:"RequestId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return cloud.Operation{}, fmt.Errorf("unmarshal %s: %w", action, err)
	}
	op := cloud.NewOperation(kind, resp.RequestId, time.Now().UTC())
	op.NativeID = id
	return op, nil
}

// GetOperation 轮询推进操作状态。ECS 无任务查询 API，以目标实例状态推断：
// create=实例出现即成功；start=Running；stop=Stopped；delete=查不到即成功。
// 未到完成态输出 running。ECS 的失败同步发生在请求时（do 非 200 即报错），
// 本面不制造 failed 态。
func (p *Provider) GetOperation(ctx context.Context, op cloud.Operation) (cloud.Operation, error) {
	if op.IsTerminal() {
		return op, nil
	}
	if op.Kind != cloud.OperationCreate && op.Kind != cloud.OperationStart &&
		op.Kind != cloud.OperationStop && op.Kind != cloud.OperationDelete {
		return op, fmt.Errorf("ecs get operation: unknown operation kind %q", op.Kind)
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
		done = found && it.Status == "Running"
	case cloud.OperationStop:
		done = found && it.Status == "Stopped"
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
