package protocol

import (
	"strconv"
	"strings"
)

// CompareVersion 比较两个能力版本串（semver-lite），返回值 -1/0/1 分别表示
// a<b / a==b / a>b。
//
// 规则（P0 Agent 契约首片，见 docs/design/agent-capability-contract.md §4.2
// 「版本无契约语义」缺口）：
//   - 按 '.' 分段逐段比较；数值段按整数比较（"1.10" > "1.9"）；
//   - 段数少的缺失段视为 "0"（"1" == "1.0.0"）；
//   - 空串视为 "0"（版本缺失的最低位）；
//   - 开头的 'v' 前缀忽略；
//   - 段非纯数值时按字符串字典序比较（semver-lite：不实现预发布
//     pre-release 优先级，也不解析 build metadata）。
func CompareVersion(a, b string) int {
	a = strings.TrimSpace(strings.TrimPrefix(a, "v"))
	b = strings.TrimSpace(strings.TrimPrefix(b, "v"))
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := "0", "0"
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if cmp := compareSegment(av, bv); cmp != 0 {
			return cmp
		}
	}
	return 0
}

// VersionSatisfies 报告 actual 是否满足最低版本要求（actual >= min）。
// min 为空表示不校验版本（与 CapabilitySatisfies 的调用方语义对齐）。
func VersionSatisfies(actual, min string) bool {
	if min == "" {
		return true
	}
	return CompareVersion(actual, min) >= 0
}

// compareSegment 比较单个版本段：两侧都是纯数值时按整数比较，否则按
// 字符串字典序（保持全序稳定，不返回错误）。
func compareSegment(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}