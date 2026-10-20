package agent

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/oob/liveness"
)

// parseLivenessTargets 解析探活目标清单：逗号分隔的 IP（TCP 探测端口由
// -liveness-ports 统一给出，避免「IP:端口」歧义与 IPv6 字面量冲突）。
func parseLivenessTargets(input string) ([]liveness.Target, error) {
	out := []liveness.Target{}
	for _, part := range strings.Split(input, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if net.ParseIP(item) == nil {
			return nil, fmt.Errorf("invalid liveness target %q: want an IP address", item)
		}
		out = append(out, liveness.Target{IP: item})
	}
	return out, nil
}

// parseLivenessPorts 解析端口清单：逗号分隔，每项 1..65535。
func parseLivenessPorts(input string) ([]int, error) {
	out := []int{}
	for _, part := range strings.Split(input, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		port, err := strconv.Atoi(item)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid liveness port %q: want 1..65535", item)
		}
		out = append(out, port)
	}
	return out, nil
}
