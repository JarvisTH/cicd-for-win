//go:build !tray

package serve

import (
	"testing"
)

// TestTrayStub_NoPanic 验证非 tray 编译时，空实现不会 panic。
func TestTrayStub_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tray stub should not panic: %v", r)
		}
	}()
	RunTray("", nil)
	InitTray("8080")
}
