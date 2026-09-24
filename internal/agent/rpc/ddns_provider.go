package rpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ============ DDNS Provider ============
//
// 公网出口 IP 探测（见 docs/guide/ddns-design.md D3/D4）：出站访问
// IP 回显服务，按序尝试取首个成功。只答「本机所在网络的出口 IP」，
// 配置与 DNS 写在 server（D1）。capability ddns 恒上报（标准库实现
// 无外部依赖，与 file capability 同则）。

const (
	// ddnsSourceTimeout 单源超时（D3）
	ddnsSourceTimeout = 5 * time.Second
	// ddnsFamilyTimeout 一族（v4/v6）总超时
	ddnsFamilyTimeout = 15 * time.Second
	// ddnsMaxBody 回显服务正常响应只有几十字节，超限即异常
	ddnsMaxBody = 256
)

// IP 回显服务源列表（按序兜底，做成变量仅为测试可注入）
var (
	ddnsIPv4Sources = []string{
		"https://api.ipify.org",
		"https://api-ipv4.ip.sb",
		"https://ipv4.icanhazip.com",
		"https://4.ipw.cn",
	}
	ddnsIPv6Sources = []string{
		"https://api6.ipify.org",
		"https://api-ipv6.ip.sb",
		"https://ipv6.icanhazip.com",
		"https://6.ipw.cn",
	}
	// ddnsFamilyContext 族总超时的 ctx 构造（测试注入短超时覆盖
	// ctx.Done 分支，默认实现行为不变）
	ddnsFamilyContext = func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), ddnsFamilyTimeout)
	}
	// ddnsFetchOne 单源探测（测试可替换为挂起/固定响应）
	ddnsFetchOne = (*DDNSProvider).fetchOne
)

// DDNSProvider 公网 IP 探测
type DDNSProvider struct {
	client *http.Client
}

// NewDDNSProvider 创建 provider；client 为 nil 时使用默认超时的 http.Client
func NewDDNSProvider(client *http.Client) *DDNSProvider {
	if client == nil {
		client = &http.Client{Timeout: ddnsSourceTimeout}
	}
	return &DDNSProvider{client: client}
}

// Type RPC provider 类型
func (p *DDNSProvider) Type() string { return "ddns" }

// Call RPC 分发
func (p *DDNSProvider) Call(action string, _ map[string]interface{}) (interface{}, error) {
	if action != "ip" {
		return nil, fmt.Errorf("unsupported action %q", action)
	}
	return p.probeIPs()
}

// probeIPs 探测 IPv4/IPv6 出口 IP，取不到的族省略（D4，不报错）
func (p *DDNSProvider) probeIPs() (interface{}, error) {
	result := map[string]interface{}{}
	if ip := p.probeFamily(ddnsIPv4Sources, false); ip != "" {
		result["ipv4"] = ip
	}
	if ip := p.probeFamily(ddnsIPv6Sources, true); ip != "" {
		result["ipv6"] = ip
	}
	return result, nil
}

// probeFamily 一族源按序尝试，首个通过校验的响应生效
func (p *DDNSProvider) probeFamily(sources []string, wantV6 bool) string {
	ctx, cancel := ddnsFamilyContext()
	defer cancel()

	for _, src := range sources {
		select {
		case <-ctx.Done():
			return "" // 族总超时，后续源不再尝试
		default:
		}
		if ip := ddnsFetchOne(p, ctx, src, wantV6); ip != "" {
			return ip
		}
	}
	return ""
}

// fetchOne 请求单个回显服务并校验响应（D12：net.ParseIP + 地址族匹配）
func (p *DDNSProvider) fetchOne(ctx context.Context, src string, wantV6 bool) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return ""
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ddnsMaxBody))
	if err != nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(string(body)))
	if ip == nil {
		return ""
	}
	if wantV6 != (ip.To4() == nil) {
		return "" // 地址族不匹配（回显服务配置错误的兜底）
	}
	return ip.String()
}
