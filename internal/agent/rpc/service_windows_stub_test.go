//go:build !windows

package rpc

import "testing"

// stub 行为（service-design.md D9.5）：非 Windows 平台构造的 windows-scm
// provider 一律显式报错；正常路径探测门控根本不会注册它（防御层）
func TestWindowsServiceStubRejects(t *testing.T) {
	p := NewWindowsServiceProvider()
	if p.Type() != "service" {
		t.Errorf("Type() = %q, want service", p.Type())
	}
	for _, action := range []string{"list", "status", "action"} {
		if _, err := p.Call(action, nil); err == nil {
			t.Errorf("stub Call(%q) should error on non-windows build", action)
		}
	}
}
