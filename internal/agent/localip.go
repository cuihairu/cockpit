package agent

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// interfaceAddrs / publicIPEndpoints 覆盖率注入点（同 agent.go goos 范式）：
// 未导出包级 var，默认值即生产取值，行为不变；同包测试注入本地
// httptest/不可达地址覆盖错误与空响应分支，注入前后 defer 恢复。
// 读方 localIPs/publicIP 仅由 register（测试 goroutine 同步调用）与直调
// 测试触达；agent 后台循环不读它（Stop 后 reconnect 先查 ctx 直接退出，
// 不会再进 register），同包测试串行执行，无数据竞争。
var (
	interfaceAddrs    = net.InterfaceAddrs
	publicIPEndpoints = []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
)

// localIPs 返回本机所有非 loopback 的 IPv4 地址（局域网 IP）。
func localIPs() []string {
	addrs, err := interfaceAddrs()
	if err != nil {
		return nil
	}
	var ips []string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			ips = append(ips, ip4.String())
		}
	}
	return ips
}

// publicIP 返回本机公网 IPv4 地址（通过外部服务探测）。
// 失败时返回空字符串，注册仍继续。
func publicIP() string {
	client := &http.Client{Timeout: 3 * time.Second}

	for _, endpoint := range publicIPEndpoints {
		resp, err := client.Get(endpoint)
		if err != nil {
			log.Printf("publicIP: %s: %v", endpoint, err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip
		}
	}

	log.Printf("publicIP: all endpoints failed")
	return ""
}
