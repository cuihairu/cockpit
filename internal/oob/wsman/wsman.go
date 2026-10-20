// Package wsman 提供 WS-Management Identify 探测（DMTF WS-Management
// Identity 1.0：免认证的单轮 SOAP 问答），AMT/DASH 探测线共用。
// 边界：本包只做 Identify 发现面；操作面（SOAP Enumerate/Get/Put，需认证）
// 随各厂商操作片落位，不在本包预设。
package wsman

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// identifyAction 是 WS-Management Identity 1.0 的 Identify 动作 URI。
const identifyAction = "http://schemas.dmtf.org/wbem/wsman/identity/1/wsidentity"

// identifyRequest 是 SOAP 1.2 + WS-Addressing 的 Identify 请求信封
// （空 Body，动作在 Header；实现按 2005/08 寻址命名空间发，兼容 2004/08 端点）。
const identifyRequest = `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing"><s:Header><a:Action>` + identifyAction + `</a:Action></s:Header><s:Body/></s:Envelope>`

// identifyTimeout 单轮 Identify 的默认超时。
const identifyTimeout = 5 * time.Second

// Options 探测客户端配置。
type Options struct {
	// Client 可注入（测试用 httptest 服务器走 http）；nil 用默认客户端。
	Client *http.Client
}

// Client WS-Management Identify 客户端。
type Client struct {
	client *http.Client
}

// New 构造 Identify 客户端。
func New(opts Options) *Client {
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: identifyTimeout}
	}
	return &Client{client: opts.Client}
}

// IdentifyResponse 只取发现面字段（产品厂商标识由各 Provider 自行判定匹配）。
type IdentifyResponse struct {
	ProductVendor   string
	ProductName     string
	ProductVersion  string
	ProtocolVersion string
}

// decode 按本地名解码（命名空间前缀漂移不影响：各实现用 s:/a:/w: 不一）。
type decode struct {
	Body struct {
		IdentifyResponse struct {
			ProductVendor   string `xml:"ProductVendor"`
			ProductName     string `xml:"ProductName"`
			ProductVersion  string `xml:"ProductVersion"`
			ProtocolVersion string `xml:"ProtocolVersion"`
		} `xml:"IdentifyResponse"`
	} `xml:"Body"`
}

// Identify 对 baseURL 发一次免认证 Identify。
// status==0 表示传输层失败（err 非 nil）；非 2xx 返回状态码与错误；
// 2xx 但不是 IdentifyResponse 文档（无任何产品字段）返回状态码与解码错误。
func (c *Client) Identify(ctx context.Context, baseURL string) (IdentifyResponse, int, error) {
	url := strings.TrimRight(baseURL, "/") + "/wsman"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(identifyRequest)))
	if err != nil {
		return IdentifyResponse{}, 0, err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=UTF-8")

	resp, err := c.client.Do(req)
	if err != nil {
		return IdentifyResponse{}, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return IdentifyResponse{}, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return IdentifyResponse{}, resp.StatusCode, fmt.Errorf("wsman: endpoint status %d", resp.StatusCode)
	}

	var doc decode
	if err := xml.Unmarshal(body, &doc); err != nil {
		return IdentifyResponse{}, resp.StatusCode, fmt.Errorf("wsman: not a wsman identify response: %w", err)
	}
	ir := doc.Body.IdentifyResponse
	if ir.ProductName == "" && ir.ProductVendor == "" && ir.ProductVersion == "" && ir.ProtocolVersion == "" {
		return IdentifyResponse{}, resp.StatusCode, fmt.Errorf("wsman: 2xx body lacks IdentifyResponse product fields, not a wsman endpoint")
	}
	return IdentifyResponse{
		ProductVendor:   ir.ProductVendor,
		ProductName:     ir.ProductName,
		ProductVersion:  ir.ProductVersion,
		ProtocolVersion: ir.ProtocolVersion,
	}, resp.StatusCode, nil
}
