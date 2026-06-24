//go:build windows

package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogMsg_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("logMsg should not panic: %v", r)
		}
	}()
	logMsg("test message")
	logMsg("")
}

func TestTrayLog_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("trayLog should not panic: %v", r)
		}
	}()
	// 设置 apiBase
	apiBase = "http://localhost:18080"
	trayLog("test-proj", "check", "pass", "")
	trayLog("test-proj", "build", "fail", "build error details")
}

func TestApiGet_WithServer(t *testing.T) {
	// 创建测试 HTTP 服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	apiBase = ts.URL
	resp := apiGet("/test")
	if resp == "" {
		t.Error("apiGet should return response body")
	}
}

func TestApiPostJSON_WithServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	apiBase = ts.URL
	resp := apiPostJSON("/test", nil)
	if resp == "" {
		t.Error("apiPostJSON should return response body")
	}
}

func TestApiGet_ServerDown(t *testing.T) {
	// 指向不存在的服务器，验证不 panic 且返回空
	apiBase = "http://localhost:1"
	resp := apiGet("/test")
	if resp != "" {
		t.Error("apiGet should return empty when server is down")
	}
}
