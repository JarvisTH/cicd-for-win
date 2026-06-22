// Package util 提供 CI/CD 工具链内部共享的通用工具函数，消除包间代码重复。
package util

import (
	"os"
	"path/filepath"
	"runtime"
)

// LocalParent 返回路径的上一级目录。已在根目录时返回空字符串。
// 同时处理 Windows 盘符根（C:\ → ""）和 Unix 根（/ → ""）。
func LocalParent(p string) string {
	clean := filepath.Clean(p)
	parent := filepath.Dir(clean)
	if runtime.GOOS == "windows" {
		if len(clean) == 3 && clean[1] == ':' && (clean[2] == '\\' || clean[2] == '/') {
			return ""
		}
		if len(parent) == 2 && parent[1] == ':' {
			return ""
		}
	} else if clean == "/" {
		return ""
	}
	if parent == clean {
		return ""
	}
	return parent
}

// ListDrives 枚举 A-Z 可用盘符（仅 Windows 有意义，其他平台返回空切片）。
func ListDrives() []string {
	var drives []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			drives = append(drives, root)
		}
	}
	return drives
}
