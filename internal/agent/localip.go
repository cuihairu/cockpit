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
// httptest/不可达地址覆盖错误与空响应分支，注入/恢复经 injectSwap。
// 读方 localIPs/publicIP 经 injectRead：register 的 Facts 闭包跑在
// reconnect 后台 goroutine（测试亦直接 go a.reconnect()），Stop 只 cancel
// 不 join 循环，测试 goroutine 的恢复写与在途 register 的读可并发——
// CI race 实测（2026-11-08 main 红），守卫与理据见 inject.go。
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
	addrs, err := injectRead(&interfaceAddrs)()
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

	for _, endpoint := range injectRead(&publicIPEndpoints) {
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
