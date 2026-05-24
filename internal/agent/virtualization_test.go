package agent

import (
	"testing"
)

func TestVirtualizationTypeConstants(t *testing.T) {
	types := []VirtualizationType{
		VirtTypeNone,
		VirtTypeKVM,
		VirtTypeVMware,
		VirtTypeVirtualBox,
		VirtTypeQEMU,
		VirtTypeXen,
		VirtTypeHyperV,
		VirtTypeParallels,
		VirtTypeContainer,
		VirtTypeOpenVZ,
		VirtTypeLXC,
		VirtTypeDocker,
	}

	for _, vt := range types {
		if vt == "" {
			t.Errorf("VirtualizationType constant should not be empty")
		}
	}
}

func TestSystemRoleConstants(t *testing.T) {
	roles := []SystemRole{
		RoleGuest,
		RoleHost,
	}

	for _, role := range roles {
		if role == "" {
			t.Errorf("SystemRole constant should not be empty")
		}
	}
}

func TestVirtualizationInfo(t *testing.T) {
	info := &VirtualizationInfo{
		Type:     VirtTypeKVM,
		Role:     RoleGuest,
		Platform: "qemu",
	}

	if info.Type != VirtTypeKVM {
		t.Errorf("Type = %v, want %v", info.Type, VirtTypeKVM)
	}

	if info.Role != RoleGuest {
		t.Errorf("Role = %v, want %v", info.Role, RoleGuest)
	}

	if info.Platform != "qemu" {
		t.Errorf("Platform = %v, want 'qemu'", info.Platform)
	}
}

func TestVirtualizationInfoWithEmptyPlatform(t *testing.T) {
	info := &VirtualizationInfo{
		Type: VirtTypeNone,
		Role: RoleHost,
	}

	if info.Platform != "" {
		t.Errorf("Platform should be empty, got %v", info.Platform)
	}
}

func TestIsVirtualMachine(t *testing.T) {
	// This test will use the actual detection result
	// We just verify the function runs without panic
	result := IsVirtualMachine()
	// Result could be true or false depending on the environment
	_ = result
}

func TestIsContainer(t *testing.T) {
	// This test will use the actual detection result
	result := IsContainer()
	_ = result
}

func TestIsPhysicalMachine(t *testing.T) {
	// This test will use the actual detection result
	result := IsPhysicalMachine()
	_ = result
}

func TestDetectVirtualization(t *testing.T) {
	// This test will use the actual detection result
	info := DetectVirtualization()
	if info == nil {
		t.Error("DetectVirtualization() should not return nil")
		return
	}

	// Verify that the result has valid values
	validTypes := map[VirtualizationType]bool{
		VirtTypeNone:       true,
		VirtTypeKVM:        true,
		VirtTypeVMware:     true,
		VirtTypeVirtualBox: true,
		VirtTypeQEMU:       true,
		VirtTypeXen:        true,
		VirtTypeHyperV:     true,
		VirtTypeParallels:  true,
		VirtTypeContainer:  true,
		VirtTypeOpenVZ:     true,
		VirtTypeLXC:        true,
		VirtTypeDocker:     true,
	}

	if !validTypes[info.Type] {
		t.Errorf("Invalid Type: %v", info.Type)
	}

	validRoles := map[SystemRole]bool{
		RoleGuest: true,
		RoleHost:  true,
	}

	if !validRoles[info.Role] {
		t.Errorf("Invalid Role: %v", info.Role)
	}
}

func TestDetectVirtualizationCached(t *testing.T) {
	// Call multiple times, should return the same cached result
	info1 := DetectVirtualization()
	info2 := DetectVirtualization()

	if info1 != info2 {
		t.Error("DetectVirtualization() should return cached result on second call")
	}
}

func TestIsContainerMutualExclusivity(t *testing.T) {
	// If running in a container, IsVirtualMachine should return false for container types
	if IsContainer() {
		vm := IsVirtualMachine()
		if vm {
			t.Error("Should not be both a container and a virtual machine")
		}
	}
}

func TestIsPhysicalMachineNotGuest(t *testing.T) {
	// Physical machine should not be a guest
	if IsPhysicalMachine() {
		if IsContainer() {
			t.Error("Physical machine should not be detected as container")
		}
	}
}

func TestVirtualizationTypeString(t *testing.T) {
	tests := []struct {
		vt       VirtualizationType
		expected string
	}{
		{VirtTypeNone, "none"},
		{VirtTypeKVM, "kvm"},
		{VirtTypeVMware, "vmware"},
		{VirtTypeVirtualBox, "virtualbox"},
		{VirtTypeQEMU, "qemu"},
		{VirtTypeXen, "xen"},
		{VirtTypeHyperV, "hyperv"},
		{VirtTypeParallels, "parallels"},
		{VirtTypeContainer, "container"},
		{VirtTypeOpenVZ, "openvz"},
		{VirtTypeLXC, "lxc"},
		{VirtTypeDocker, "docker"},
	}

	for _, tt := range tests {
		if string(tt.vt) != tt.expected {
			t.Errorf("VirtualizationType string = %v, want %v", tt.vt, tt.expected)
		}
	}
}

func TestSystemRoleString(t *testing.T) {
	tests := []struct {
		role     SystemRole
		expected string
	}{
		{RoleGuest, "guest"},
		{RoleHost, "host"},
	}

	for _, tt := range tests {
		if string(tt.role) != tt.expected {
			t.Errorf("SystemRole string = %v, want %v", tt.role, tt.expected)
		}
	}
}
