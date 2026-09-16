package detector

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&PVEDetector{})
}

// pveDefaultEndpoints 无 PVE_URL 时的候选地址。做成包级变量仅为测试
// 可注入（cov_* 测试指向 httptest 服务），默认值即生产取值。
var pveDefaultEndpoints = []string{
	"https://127.0.0.1:8006",
	"https://192.168.1.10:8006",
	"https://192.168.0.10:8006",
}

// PVEDetector PVE API 检测器
type PVEDetector struct{}

// Name 检测器名称
func (d *PVEDetector) Name() string {
	return "pve-api"
}

// Priority 检测优先级
func (d *PVEDetector) Priority() int {
	return 10
}

// Detect 检测 PVE API
func (d *PVEDetector) Detect() (*protocol.Capability, error) {
	// 1. 检查环境变量
	url := os.Getenv("PVE_URL")
	if url == "" {
		// 尝试常见内网地址
		for _, candidate := range pveDefaultEndpoints {
			if d.testAPI(candidate) {
				return &protocol.Capability{
					Type:     "pve-api",
					Endpoint: candidate,
					Version:  "detected",
				}, nil
			}
		}
		return nil, nil
	}

	// 2. 测试指定的 URL
	if d.testAPI(url) {
		// 获取版本信息
		version := d.getVersion(url)
		return &protocol.Capability{
			Type:     "pve-api",
			Endpoint: url,
			Version:  version,
		}, nil
	}

	return nil, nil
}

// testAPI 测试 PVE API 是否可访问
func (d *PVEDetector) testAPI(url string) bool {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(url + "/api2/json/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

// getVersion 获取 PVE 版本
func (d *PVEDetector) getVersion(url string) string {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(url + "/api2/json/version")
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "unknown"
	}

	return result.Data.Version
}
