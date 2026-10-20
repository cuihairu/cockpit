package liveness

import (
	"os"
	"strings"
)

// procNetARP Linux 内核邻居表视图。
const procNetARP = "/proc/net/arp"

// Neighbor 邻居表条目。
type Neighbor struct {
	IP    string
	MAC   string
	State string // complete / incomplete
	Dev   string
}

// readNeighbors 邻居表读取注入点：默认读 /proc/net/arp（Linux）。测试注入
// 临时文件后可不依赖 CI 机器的邻居表内容。
var readNeighbors = func() ([]Neighbor, error) {
	return readARPFile(procNetARP)
}

// readARPFile 解析 /proc/net/arp 形态：表头一行 + 「IP 类型 flags MAC mask 设备」
// 六列；flags 0x0 或 MAC 全零表示条目未解析（incomplete）。
func readARPFile(path string) ([]Neighbor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := []Neighbor{}
	for _, line := range strings.Split(string(data), "\n")[1:] { // 跳过表头
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		mac := strings.TrimSpace(fields[3])
		state := "complete"
		if mac == "00:00:00:00:00:00" {
			mac = ""
		}
		if fields[2] == "0x0" {
			state = "incomplete"
		}
		out = append(out, Neighbor{IP: fields[0], MAC: mac, State: state, Dev: fields[5]})
	}
	return out, nil
}

// probeARP 由邻居表判定在位：条目带 MAC → present（缓存态同样是在位证据）；
// 无条目、条目未解析或表不可读 → unknown（探测不到 ≠ 不存在）。
func probeARP(ip string) Source {
	neighbors, err := readNeighbors()
	if err != nil {
		return Source{Verdict: VerdictUnknown, Detail: "arp: neighbor table unavailable"}
	}
	for _, n := range neighbors {
		if n.IP != ip {
			continue
		}
		if n.MAC == "" {
			return Source{Verdict: VerdictUnknown, Detail: "arp: entry incomplete"}
		}
		return Source{
			Verdict: VerdictPresent,
			Detail:  "arp " + n.State + " " + n.MAC + " on " + n.Dev,
		}
	}
	return Source{Verdict: VerdictUnknown, Detail: "arp: no neighbor entry"}
}
