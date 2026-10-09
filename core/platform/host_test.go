package platform

import (
	"os"
	"testing"
)

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
	if got.Paths() != (Paths{ConfigDir: "c", DataDir: "d", LogDir: "l"}) {
		t.Fatalf("Current().Paths() = %+v", got.Paths())
	}
	if len(got.Signals()) != 1 {
		t.Fatalf("Current().Signals() = %v", got.Signals())
	}
}

// TestSelectAssembly 本测试二进制含 select_<goos>.go：编译期装配后
// Current 恒非 nil（四互斥全覆盖 GOOS），且 Paths/Signals 由平台实现
// 填充（生产事实非空）。
func TestSelectAssembly(t *testing.T) {
	h := Current()
	if h == nil {
		t.Fatal("platform.Current() should be assembled by select_<goos>.go")
	}
	p := h.Paths()
	if p.ConfigDir == "" || p.DataDir == "" || p.LogDir == "" {
		t.Fatalf("assembled Paths has empty field: %+v", p)
	}
	if len(h.Signals()) == 0 {
		t.Fatal("assembled Signals should not be empty")
	}
}

type fakeHost struct{}

func (fakeHost) MachineID() string { return "fake-id" }

func (fakeHost) Paths() Paths { return Paths{ConfigDir: "c", DataDir: "d", LogDir: "l"} }

func (fakeHost) Signals() []os.Signal { return []os.Signal{os.Interrupt} }
