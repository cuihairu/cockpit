package liveness

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

// pingHost ICMP 探测注入点：默认调系统 ping（BusyBox ping 同参数兼容，
// OpenWrt 侧可用）。测试注入桩后零真发报文。
var pingHost = func(ctx context.Context, ip string, timeout time.Duration) error {
	secs := int64(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.FormatInt(secs, 10), ip).Run()
}

// probeICMP 由 ICMP 应答判定在位：echo 应答 → present；超时/不可达/无
// ping 二进制 → unknown（无应答不等于关机，见三态纪律）。
func probeICMP(ctx context.Context, ip string, timeout time.Duration) Source {
	if err := pingHost(ctx, ip, timeout); err != nil {
		return Source{
			Verdict: VerdictUnknown,
			Detail:  "icmp: no echo reply",
		}
	}
	return Source{Verdict: VerdictPresent, Detail: "icmp echo reply"}
}
