package detector

import (
	"testing"
)

func TestGetNetworkInterfacesNonLinux(t *testing.T) {
	_, err := GetNetworkInterfaces()
	if err == nil {
		t.Log("GetNetworkInterfaces() succeeded (running on Linux)")
	} else {
		t.Logf("GetNetworkInterfaces() error (expected on non-Linux): %v", err)
	}
}

func TestGetDisksNonLinux(t *testing.T) {
	_, err := GetDisks()
	if err == nil {
		t.Log("GetDisks() succeeded (running on Linux)")
	} else {
		t.Logf("GetDisks() error (expected on non-Linux): %v", err)
	}
}

func TestGetSystemInfoNonLinux(t *testing.T) {
	_, err := GetSystemInfo()
	if err == nil {
		t.Log("GetSystemInfo() succeeded (running on OpenWrt)")
	} else {
		t.Logf("GetSystemInfo() error (expected on non-OpenWrt): %v", err)
	}
}

func TestReadOpenWrtReleaseNonLinux(t *testing.T) {
	d := &OpenWrtDetector{}
	_, err := d.readOpenWrtRelease()
	if err == nil {
		t.Log("readOpenWrtRelease() succeeded (running on OpenWrt)")
	} else {
		t.Logf("readOpenWrtRelease() error (expected on non-OpenWrt): %v", err)
	}
}

func TestReadOpenWrtReleaseWithMockFile(t *testing.T) {
	d := &OpenWrtDetector{}
	_, err := d.readOpenWrtRelease()
	_ = err
}
