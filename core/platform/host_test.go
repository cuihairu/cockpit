package platform

import "testing"

// TestRegisterAndCurrent 注册/取用闭环（生产由 select_<goos>.go init 注册）。
func TestRegisterAndCurrent(t *testing.T) {
	prev := Current()
	t.Cleanup(func() { Register(prev) })

	fake := fakeHost{}
	Register(fake)
	got := Current()
	if got.MachineID() != "fake-id" {
		t.Fatalf("Current().MachineID() = %q", got.MachineID())
	}
}

// TestSelectAssembly 本测试二进制含 select_<goos>.go：编译期装配后
// Current 恒非 nil（四互斥全覆盖 GOOS）。
func TestSelectAssembly(t *testing.T) {
	if Current() == nil {
		t.Fatal("platform.Current() should be assembled by select_<goos>.go")
	}
}

type fakeHost struct{}

func (fakeHost) MachineID() string { return "fake-id" }
