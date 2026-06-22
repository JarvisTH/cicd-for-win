package serve

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"ci-cd/internal/config"
)

// 密码修改频率限制：同一 IP 5 分钟内最多尝试 5 次
var (
	loginAttempts   = map[string][]time.Time{}
	loginMu         sync.Mutex
	maxLoginAttempt = 5
	loginWindow     = 5 * time.Minute
)

func checkLoginRateLimit(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	// 清理过期记录
	attempts, ok := loginAttempts[ip]
	if !ok {
		loginAttempts[ip] = []time.Time{now}
		return true
	}
	var recent []time.Time
	for _, t := range attempts {
		if now.Sub(t) < loginWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= maxLoginAttempt {
		loginAttempts[ip] = recent
		return false
	}
	recent = append(recent, now)
	loginAttempts[ip] = recent
	return true
}

// authStatusHandler 返回当前认证状态（不暴露密码）
func authStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	auth := getActiveAuth()
	if auth == nil {
		respondJSON(w, 200,map[string]any{"username": "", "is_default": false})
		return
	}
	isDefault := auth.Username == config.DefaultUsername && auth.VerifyPassword(config.DefaultPassword)
	respondJSON(w, 200,map[string]any{
		"username":   auth.Username,
		"is_default": isDefault,
	})
}

// changePasswordHandler 处理密码修改请求
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 频率限制：同一 IP 限制尝试次数
	ip := r.RemoteAddr
	if !checkLoginRateLimit(ip) {
		log.Printf("⚠️ 密码修改过于频繁 (IP: %s)\n", ip)
		respondJSON(w, 200,map[string]string{"error": "操作过于频繁，请稍后再试"})
		return
	}

	ciDir := findCiDir()
	if ciDir == "" {
		respondJSON(w, 200,map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, 200,map[string]string{"error": "请求格式错误"})
		return
	}

	if body.OldPassword == "" || body.NewPassword == "" {
		respondJSON(w, 200,map[string]string{"error": "旧密码和新密码不能为空"})
		return
	}

	if len(body.NewPassword) < 6 {
		respondJSON(w, 200,map[string]string{"error": "新密码长度不能少于 6 位"})
		return
	}

	auth := getActiveAuth()
	if auth == nil {
		respondJSON(w, 200,map[string]string{"error": "认证未初始化"})
		return
	}

	// 验证旧密码
	if !auth.VerifyPassword(body.OldPassword) {
		respondJSON(w, 200,map[string]string{"error": "旧密码错误"})
		return
	}

	// 生成新配置并保存
	newAuth := config.NewAuthConfig(auth.Username, body.NewPassword)
	if err := config.SaveAuth(ciDir, newAuth); err != nil {
		respondJSON(w, 200,map[string]string{"error": "保存密码失败: " + err.Error()})
		return
	}

	// 更新内存缓存
	setActiveAuth(newAuth)

	log.Printf("🔑 密码已修改 (用户: %s)\n", auth.Username)
	respondJSON(w, 200,map[string]string{"status": "ok", "message": "密码修改成功"})
}
