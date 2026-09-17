package rpc

import (
	"testing"
)

// lsblk fixture：一块 SATA 盘 + 一块 NVMe + 一个分区 + 一个 loop（后两者应被过滤）
const lsblkFixture = `{
  "blockdevices": [
    {"name":"/dev/sda","type":"disk","size":500107862016,"model":"Samsung SSD 870","serial":"S6PXNZ0R123456"},
    {"name":"/dev/nvme0n1","type":"disk","size":1000204886016,"model":"WD Blue SN570","serial":"21BPJ0ABC"},
    {"name":"/dev/sda1","type":"part","size":524288000,"model":null,"serial":null},
    {"name":"/dev/loop0","type":"loop","size":4096,"model":null,"serial":null}
  ]
}`

func TestParseLsblkDisks(t *testing.T) {
	disks := parseLsblkDisks([]byte(lsblkFixture))
	if len(disks) != 2 {
		t.Fatalf("len(disks) = %d, want 2 (只留 TYPE=disk)", len(disks))
	}
	if disks[0].Name != "/dev/sda" || disks[0].Model != "Samsung SSD 870" || disks[0].Serial != "S6PXNZ0R123456" {
		t.Errorf("disks[0] = %+v", disks[0])
	}
	if disks[0].SizeBytes != 500107862016 {
		t.Errorf("SizeBytes = %d, want 500107862016", disks[0].SizeBytes)
	}
	if disks[1].Name != "/dev/nvme0n1" {
		t.Errorf("disks[1].Name = %q", disks[1].Name)
	}
}

func TestParseLsblkDisksBadInput(t *testing.T) {
	if got := parseLsblkDisks([]byte("not json")); got != nil {
		t.Errorf("bad json should return nil, got %v", got)
	}
	if got := parseLsblkDisks([]byte(`{}`)); len(got) != 0 {
		t.Errorf("empty blockdevices should return empty, got %v", got)
	}
}

// ATA 盘 smartctl fixture：passed + 全部关键属性
const smartctlATAFixture = `{
  "device": {"name": "/dev/sda", "type": "sat"},
  "smart_status": {"passed": true},
  "temperature": {"current": 34},
  "ata_smart_attributes": {"table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "value": 100, "worst": 100, "thresh": 10, "raw": {"value": 0, "string": "0"}},
    {"id": 9, "name": "Power_On_Hours", "value": 90, "worst": 90, "thresh": 0, "raw": {"value": 12345, "string": "12345"}},
    {"id": 194, "name": "Temperature_Celsius", "value": 68, "worst": 54, "thresh": 0, "raw": {"value": 34, "string": "34"}},
    {"id": 197, "name": "Current_Pending_Sector", "value": 100, "worst": 100, "thresh": 0, "raw": {"value": 0, "string": "0"}}
  ]}
}`

func TestParseSmartctlATAPassed(t *testing.T) {
	base := lsblkDisk{Name: "/dev/sda", Model: "Samsung SSD 870", Serial: "S6PX", SizeBytes: 500107862016}
	dev := parseSmartctlDevice(base, []byte(smartctlATAFixture))
	if dev.Health != smartHealthPassed {
		t.Errorf("Health = %q, want passed", dev.Health)
	}
	if dev.TemperatureC != 34 {
		t.Errorf("TemperatureC = %d, want 34", dev.TemperatureC)
	}
	if dev.PowerOnHours == nil || *dev.PowerOnHours != 12345 {
		t.Errorf("PowerOnHours = %v, want 12345", dev.PowerOnHours)
	}
	if dev.ReallocatedSectors == nil || *dev.ReallocatedSectors != 0 {
		t.Errorf("ReallocatedSectors = %v, want 0", dev.ReallocatedSectors)
	}
	if dev.PendingSectors == nil || *dev.PendingSectors != 0 {
		t.Errorf("PendingSectors = %v, want 0", dev.PendingSectors)
	}
	if dev.Error != "" {
		t.Errorf("Error = %q, want empty", dev.Error)
	}
}

// failed 盘：smart_status.passed=false + 待定扇区非 0（raw.value 字符串形态）
const smartctlATAFailedFixture = `{
  "device": {"name": "/dev/sdb", "type": "sat"},
  "smart_status": {"passed": false},
  "temperature": {"current": 41},
  "ata_smart_attributes": {"table": [
    {"id": 5, "name": "Reallocated_Sector_Ct", "raw": {"value": "48", "string": "48"}},
    {"id": 197, "name": "Current_Pending_Sector", "raw": {"value": "8", "string": "8"}}
  ]}
}`

func TestParseSmartctlATAFailed(t *testing.T) {
	dev := parseSmartctlDevice(lsblkDisk{Name: "/dev/sdb"}, []byte(smartctlATAFailedFixture))
	if dev.Health != smartHealthFailed {
		t.Errorf("Health = %q, want failed", dev.Health)
	}
	if dev.ReallocatedSectors == nil || *dev.ReallocatedSectors != 48 {
		t.Errorf("ReallocatedSectors = %v, want 48（字符串形态）", dev.ReallocatedSectors)
	}
	if dev.PendingSectors == nil || *dev.PendingSectors != 8 {
		t.Errorf("PendingSectors = %v, want 8", dev.PendingSectors)
	}
	if dev.PowerOnHours != nil {
		t.Errorf("PowerOnHours should be omitted, got %v", *dev.PowerOnHours)
	}
}

// NVMe 盘：nvme_smart_health_information_log + critical_warning 非 0
const smartctlNVMeFixture = `{
  "device": {"name": "/dev/nvme0n1", "type": "nvme"},
  "smart_status": {"passed": true, "nvme": {"critical_warning": 16}},
  "temperature": {"current": 52},
  "nvme_smart_health_information_log": {
    "critical_warning": 16,
    "media_errors": 3,
    "power_on_hours": 777,
    "percentage_used": 88
  }
}`

func TestParseSmartctlNVMe(t *testing.T) {
	dev := parseSmartctlDevice(lsblkDisk{Name: "/dev/nvme0n1"}, []byte(smartctlNVMeFixture))
	if dev.Health != smartHealthPassed {
		t.Errorf("Health = %q, want passed", dev.Health)
	}
	if dev.MediaErrors == nil || *dev.MediaErrors != 3 {
		t.Errorf("MediaErrors = %v, want 3", dev.MediaErrors)
	}
	if dev.PowerOnHours == nil || *dev.PowerOnHours != 777 {
		t.Errorf("PowerOnHours = %v, want 777", dev.PowerOnHours)
	}
	if dev.PercentUsed != 88 {
		t.Errorf("PercentUsed = %d, want 88", dev.PercentUsed)
	}
	if dev.Error == "" {
		t.Error("critical_warning 非 0 应记入 Error 提示")
	}
}

func TestParseSmartctlNoSmartStatus(t *testing.T) {
	// smartctl 能出 JSON 但无 smart_status（如权限受限、盘不支持 SMART）
	out := []byte(`{"device": {"name": "/dev/sdc", "type": "sat"}, "temperature": {"current": 30}}`)
	dev := parseSmartctlDevice(lsblkDisk{Name: "/dev/sdc"}, out)
	if dev.Health != smartHealthUnknown {
		t.Errorf("Health = %q, want unknown", dev.Health)
	}
	if dev.Error == "" {
		t.Error("unknown 时应有错误说明")
	}
}

func TestParseSmartctlBadJSON(t *testing.T) {
	dev := parseSmartctlDevice(lsblkDisk{Name: "/dev/sdd"}, []byte("garbage"))
	if dev.Health != smartHealthUnknown || dev.Name != "/dev/sdd" {
		t.Errorf("dev = %+v, want unknown + name preserved", dev)
	}
}

func TestSmartRawNumber(t *testing.T) {
	if v := smartRawNumber(float64(42)); v == nil || *v != 42 {
		t.Errorf("float64 42 => %v, want 42", v)
	}
	if v := smartRawNumber("123"); v == nil || *v != 123 {
		t.Errorf(`"123" => %v, want 123`, v)
	}
	if v := smartRawNumber("abc"); v != nil {
		t.Errorf(`"abc" => %v, want nil`, v)
	}
	if v := smartRawNumber(float64(-1)); v != nil {
		t.Errorf("-1 => %v, want nil", v)
	}
	if v := smartRawNumber(nil); v != nil {
		t.Errorf("nil => %v, want nil", v)
	}
}

func TestSmartErrSummary(t *testing.T) {
	if s := smartErrSummary([]byte("  Permission denied\n"), nil); s != "Permission denied" {
		t.Errorf("smartErrSummary = %q", s)
	}
	if s := smartErrSummary(nil, nil); s != "" {
		t.Errorf("empty should be empty, got %q", s)
	}
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	if s := smartErrSummary(long, nil); len(s) != 200 {
		t.Errorf("long summary len = %d, want 200", len(s))
	}
}
