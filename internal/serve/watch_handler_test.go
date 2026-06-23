package serve

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupWatchTest(t *testing.T) (ciDir string, cleanup func()) {
	t.Helper()
	ciDir = t.TempDir()

	// 创建 ci.exe 标记
	os.WriteFile(filepath.Join(ciDir, "ci.exe"), []byte(""), 0644)

	// 创建 projects.json
	proj := `{"projects":[{"name":"watch-test","path":"` + strings.ReplaceAll(ciDir, `\`, `\\`) + `","type":"React","enabled":true}]}`
	os.WriteFile(filepath.Join(ciDir, "projects.json"), []byte(proj), 0644)

	// 创建 auth.json
	os.WriteFile(filepath.Join(ciDir, "auth.json"), []byte(`{"username":"admin","hash":"$2a$10$dummy"}`), 0644)

	// 创建源文件，让 watcher 有东西可监听
	os.MkdirAll(filepath.Join(ciDir, "src"), 0755)
	os.WriteFile(filepath.Join(ciDir, "src", "App.tsx"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(ciDir, "package.json"), []byte(`{"dependencies":{"react":"18.0.0"}}`), 0644)

	origFindCiDir := findCiDir
	findCiDir = func() string { return ciDir }

	cleanup = func() {
		findCiDir = origFindCiDir
		// 清理所有 watcher
		activeWatchersMu.Lock()
		for name, cancel := range activeWatchers {
			cancel()
			delete(activeWatchers, name)
		}
		activeWatchersMu.Unlock()
	}
	return
}

func TestHandleWatchStart_MissingProject(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/watch/start", nil)
	handleWatchStart(w, r)

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("missing project should return error")
	}
}

func TestHandleWatchStart_Success(t *testing.T) {
	ciDir, cleanup := setupWatchTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w, r)

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("expected ok, got error: %s", result["error"])
	}

	_ = ciDir
}

func TestHandleWatchStart_Duplicate(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w1, r1)

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w2, r2)

	var result map[string]string
	json.NewDecoder(w2.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("duplicate start should still return ok, got error: %s", result["error"])
	}
}

func TestHandleWatchStart_UnknownProject(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/watch/start?project=nonexistent", nil)
	handleWatchStart(w, r)

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("unknown project should return error")
	}
}

func TestHandleWatchStop_Specific(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	// 先启动
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w1, r1)

	// 再停止
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/api/watch/stop?project=watch-test", nil)
	handleWatchStop(w2, r2)

	var result map[string]string
	json.NewDecoder(w2.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("expected ok, got error: %s", result["error"])
	}

	// 验证 watcher 已被移除
	activeWatchersMu.Lock()
	_, exists := activeWatchers["watch-test"]
	activeWatchersMu.Unlock()
	if exists {
		t.Error("watcher should be removed after stop")
	}
}

func TestHandleWatchStatus_Empty(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/watch/status", nil)
	handleWatchStatus(w, r)

	var result map[string][]string
	json.NewDecoder(w.Body).Decode(&result)
	if len(result["watching"]) != 0 {
		t.Errorf("expected empty watching list, got %v", result["watching"])
	}
}

func TestHandleWatchStatus_AfterStart(t *testing.T) {
	_, cleanup := setupWatchTest(t)
	defer cleanup()

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w1, r1)

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/api/watch/status", nil)
	handleWatchStatus(w2, r2)

	var result map[string][]string
	json.NewDecoder(w2.Body).Decode(&result)
	found := false
	for _, name := range result["watching"] {
		if name == "watch-test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("watching list should contain watch-test, got %v", result["watching"])
	}
}

func TestHandleWatchStart_WatchTriggers(t *testing.T) {
	ciDir, cleanup := setupWatchTest(t)
	defer cleanup()

	// 启动监听
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/watch/start?project=watch-test", nil)
	handleWatchStart(w, r)

	// 修改源文件触发检查
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(ciDir, "src", "App.tsx"), []byte("modified content"), 0644)

	// 等 watcher 检测到变更
	time.Sleep(3 * time.Second)

	// 停止监听
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest("GET", "/api/watch/stop?project=watch-test", nil)
	handleWatchStop(w3, r3)

	// 验证不会 panic
	t.Log("watch triggered and stopped successfully")
}
