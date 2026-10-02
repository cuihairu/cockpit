package agent

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// localIPs 返回本机所有非 loopback 的 IPv4 地址（局域网 IP）。
func localIPs() []string {
	addrs, err := net.InterfaceAddrs()
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

	for _, endpoint := range []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	} {
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