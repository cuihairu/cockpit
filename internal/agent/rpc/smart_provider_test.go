package rpc

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// smartFakeRun 可编程 Commander：按命令名返回预置输出
func smartFakeRun(lsblkOut []byte, lsblkErr error, smartOut map[string][]byte, smartErr map[string]error) Commander {
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		switch name {
		case smartLsblkBin:
			return lsblkOut, nil, lsblkErr
		case smartCtlBin:
			dev := args[len(args)-1]
			if e, ok := smartErr[dev]; ok {
				return nil, []byte("Permission denied"), e
			}
			return smartOut[dev], nil, nil
		}
		return nil, nil, errors.New("unexpected command: " + name)
	}
}

// withSmartBins 临时替换包级命令名为测试桩，并放行 LookPath 探测
func withSmartBins(t *testing.T, lsblk, smartctl string) {
	t.Helper()
	oldLsblk, oldSmart, oldLook := smartLsblkBin, smartCtlBin, smartCtlLookPath
	smartLsblkBin, smartCtlBin = lsblk, smartctl
	smartCtlLookPath = func(string) (string, error) { return smartctl, nil }
	t.Cleanup(func() {
		smartLsblkBin, smartCtlBin, smartCtlLookPath = oldLsblk, oldSmart, oldLook
	})
}

func TestSmartStatusUnavailable(t *testing.T) {
	// LookPath 找不到 smartctl → available=false（D1）
	oldLook := smartCtlLookPath
	smartCtlLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { smartCtlLookPath = oldLook })
	p := NewSmartProvider(nil)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != false {
		t.Errorf("available = %v, want false", m["available"])
	}
	if devs := m["devices"].([]smartDevice); len(devs) != 0 {
		t.Errorf("devices should be empty, got %v", devs)
	}
}

func TestSmartStatusTwoDisks(t *testing.T) {
	withSmartBins(t, "fake-lsblk", "fake-smartctl")
	run := smartFakeRun(
		[]byte(lsblkFixture), nil,
		map[string][]byte{
			"/dev/sda":     []byte(smartctlATAFixture),
			"/dev/nvme0n1": []byte(smartctlNVMeFixture),
		},
		nil,
	)
	p := NewSmartProvider(run)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	m := resp.(map[string]interface{})
	if m["available"] != true {
		t.Fatalf("available = %v, want true", m["available"])
	}
	devs := m["devices"].([]smartDevice)
	if len(devs) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devs))
	}
	if devs[0].Health != smartHealthPassed || devs[0].TemperatureC != 34 {
		t.Errorf("devs[0] = %+v", devs[0])
	}
	if devs[1].MediaErrors == nil || *devs[1].MediaErrors != 3 {
		t.Errorf("devs[1].MediaErrors = %v, want 3", devs[1].MediaErrors)
	}
}

func TestSmartStatusOneDiskFails(t *testing.T) {
	withSmartBins(t, "fake-lsblk", "fake-smartctl")
	// sda 读数失败（unknown），nvme 正常——单盘失败不影响其余（D3）
	run := smartFakeRun(
		[]byte(lsblkFixture), nil,
		map[string][]byte{"/dev/nvme0n1": []byte(smartctlNVMeFixture)},
		map[string]error{"/dev/sda": errors.New("exit status 4")},
	)
	p := NewSmartProvider(run)
	resp, _ := p.Call("status", nil)
	devs := resp.(map[string]interface{})["devices"].([]smartDevice)
	if len(devs) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devs))
	}
	if devs[0].Health != smartHealthUnknown || devs[0].Error == "" {
		t.Errorf("devs[0] = %+v, want unknown with error", devs[0])
	}
	if !strings.Contains(devs[0].Error, "Permission denied") {
		t.Errorf("Error should carry stderr summary, got %q", devs[0].Error)
	}
	if devs[1].Health != smartHealthPassed {
		t.Errorf("devs[1].Health = %q, want passed", devs[1].Health)
	}
}

func TestSmartStatusLsblkError(t *testing.T) {
	withSmartBins(t, "fake-lsblk", "fake-smartctl")
	run := smartFakeRun(nil, errors.New("lsblk exploded"), nil, nil)
	p := NewSmartProvider(run)
	_, err := p.Call("status", nil)
	if err == nil {
		t.Fatal("lsblk 失败应返回 error")
	}
}

func TestSmartStatusUnsupportedAction(t *testing.T) {
	p := NewSmartProvider(nil)
	if _, err := p.Call("write", nil); err == nil {
		t.Fatal("不支持的动作应返回 error")
	}
	if p.Type() != "hardware-monitor" {
		t.Errorf("Type() = %q, want hardware-monitor", p.Type())
	}
}

func TestSmartStatusLsblkBadJSON(t *testing.T) {
	withSmartBins(t, "fake-lsblk", "fake-smartctl")
	run := smartFakeRun([]byte("not json"), nil, nil, nil)
	p := NewSmartProvider(run)
	resp, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("坏 lsblk 输出不应致命: %v", err)
	}
	devs := resp.(map[string]interface{})["devices"].([]smartDevice)
	if len(devs) != 0 {
		t.Errorf("devices should be empty, got %v", devs)
	}
}
