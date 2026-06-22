package util

import (
	"runtime"
	"testing"
)

func TestLocalParent_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 平台测试")
	}
	tests := []struct {
		path     string
		expected string
	}{
		{`D:\project\sub`, `D:\project`},
		{`D:\project`, `D:\`},
		{`D:\`, ``},
		{`C:\`, ``},
		{`C:`, ``},
	}
	for _, tc := range tests {
		result := LocalParent(tc.path)
		if result != tc.expected {
			t.Errorf("LocalParent(%q) = %q, 期望 %q", tc.path, result, tc.expected)
		}
	}
}

func TestLocalParent_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("非 Windows 平台测试")
	}
	tests := []struct {
		path     string
		expected string
	}{
		{"/home/user/proj", "/home/user"},
		{"/home/user", "/home"},
		{"/home", "/"},
		{"/", ""},
		{"/..", ""},
	}
	for _, tc := range tests {
		result := LocalParent(tc.path)
		if result != tc.expected {
			t.Errorf("LocalParent(%q) = %q, 期望 %q", tc.path, result, tc.expected)
		}
	}
}

func TestLocalParent_Relative(t *testing.T) {
	result := LocalParent(".")
	if result == "." {
		t.Errorf("'.' 的父目录不应是自身: 得到 %q", result)
	}
}

func TestListDrives_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 平台测试")
	}
	drives := ListDrives()
	if len(drives) == 0 {
		t.Fatal("Windows 上应至少有一个盘符")
	}
	foundC := false
	for _, d := range drives {
		if len(d) != 3 || d[1] != ':' || d[2] != '\\' {
			t.Errorf("盘符格式异常: %q", d)
		}
		if d == `C:\` {
			foundC = true
		}
	}
	if !foundC {
		t.Error("应包含 C:\\")
	}
}

func TestListDrives_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("非 Windows 平台测试")
	}
	drives := ListDrives()
	if len(drives) != 0 {
		t.Errorf("非 Windows 平台应返回空切片, 得到 %v", drives)
	}
}

func TestLocalParent_NoFile(t *testing.T) {
	// 不存在的路径
	dir := t.TempDir()
	result := LocalParent(dir)
	if result == "" {
		t.Logf("临时目录 %q 的父目录: %q", dir, result)
	}
}
