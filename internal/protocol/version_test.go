package protocol

import "testing"

// TestCompareVersionTable 覆盖 semver-lite 比较的分支：等值/高低、段数
// 缺失补 0、空串视为 0、v 前缀、数值段（1.10 > 1.9）、非数值段字典序。
func TestCompareVersionTable(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"2", "1", 1},
		{"1", "2", -1},
		{"1", "1.0.0", 0},
		{"1.0.0", "1", 0},
		{"", "1", -1},
		{"1", "", 1},
		{"", "", 0},
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.4", -1},
		{"1.10", "1.9", 1},
		{"1.9", "1.10", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-x", "1.0.0-x", 0},
		{"x.1", "y.1", -1},
	}

	for _, c := range cases {
		if got := CompareVersion(c.a, c.b); got != c.want {
			t.Errorf("CompareVersion(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestVersionSatisfiesTable 覆盖最低版本校验：min 为空不校验、版本缺失
// 视为最低位、恰好等于下界通过、低于下界拒绝。
func TestVersionSatisfiesTable(t *testing.T) {
	cases := []struct {
		actual, min string
		want        bool
	}{
		{"1", "1", true},
		{"1", "2", false},
		{"2", "1", true},
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"", "1", false},
		{"", "", true},
		{"1.0.0", "", true},
		{"1.10", "1.9", true},
		{"1.9", "1.10", false},
	}

	for _, c := range cases {
		if got := VersionSatisfies(c.actual, c.min); got != c.want {
			t.Errorf("VersionSatisfies(%q, %q) = %v, want %v", c.actual, c.min, got, c.want)
		}
	}
}