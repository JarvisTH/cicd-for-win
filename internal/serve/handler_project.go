package serve

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
	"ci-cd/internal/security"
)

// findProjectPath 从 projects.json 中查找指定项目名的实际路径
func findProjectPath(ciDir, projectName string) string {
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Projects []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	for _, p := range cfg.Projects {
		if p.Name == projectName {
			return p.Path
		}
	}
	return ""
}

func findPipelineStepCommand(ciDir, projectName, stepID string) (command, args string) {
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return "", ""
	}
	var cfg struct {
		Projects []struct {
			Name     string `json:"name"`
			Pipeline *struct {
				Steps []struct {
					ID      string `json:"id"`
					Enabled bool   `json:"enabled"`
					Command string `json:"command"`
					Args    string `json:"args"`
				} `json:"steps"`
			} `json:"pipeline"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", ""
	}
	for _, p := range cfg.Projects {
		if p.Name != projectName {
			continue
		}
		if p.Pipeline == nil {
			return "", ""
		}
		for _, s := range p.Pipeline.Steps {
			if s.ID == stepID {
				return s.Command, s.Args
			}
		}
	}
	return "", ""
}

func projectListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
		return
	}

	// 解析并注入版本信息
	var cfg struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		w.Write(data)
		return
	}
	for i, p := range cfg.Projects {
		path, _ := p["path"].(string)
		if path == "" {
			continue
		}
		// 版本号
		cfg.Projects[i]["version"] = runner.ReadProjectVersion(path)
		// Git 信息
		branch, commit := runner.ReadGitInfo(path)
		cfg.Projects[i]["git_branch"] = branch
		cfg.Projects[i]["git_commit"] = commit
	}
	result, _ := json.MarshalIndent(cfg, "", "  ")
	w.Write(result)
}

func projectSaveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}

	// 强类型反序列化 + 字段校验，避免任意配置注入
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误: " + err.Error()})
		return
	}
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.Name == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目名称不能为空"})
			return
		}
		// 校验项目路径存在且是目录
		if p.Path == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目路径不能为空"})
			return
		}
		if fi, err := os.Stat(p.Path); err != nil || !fi.IsDir() {
			json.NewEncoder(w).Encode(map[string]string{"error": "项目路径不存在或不是目录: " + p.Path})
			return
		}
		// 校验并加密部署配置中的密码
		if p.Deploy != nil {
			if err := validateAndEncryptDeploy(ciDir, p.Deploy); err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}
		// 校验流水线自定义命令：必须不含 shell 元字符注入风险
		if p.Pipeline != nil {
			for _, s := range p.Pipeline.Steps {
				if !isValidStepID(s.ID) {
					json.NewEncoder(w).Encode(map[string]string{"error": "非法流水线步骤 ID: " + s.ID})
					return
				}
			}
		}
	}

	raw, _ := json.MarshalIndent(cfg, "", "  ")
	path := filepath.Join(ciDir, "projects.json")
	if err := security.AtomicWriteFile(path, raw, config.FilePermSecure); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// validateAndEncryptDeploy 校验部署配置合法性并加密明文密码。
func validateAndEncryptDeploy(ciDir string, d *config.DeployConfig) error {
	if d.Host == "" {
		return nil // 未配置部署，跳过
	}
	// 认证类型白名单
	switch d.AuthType {
	case "key", "agent", "password":
	default:
		return fmt.Errorf("非法认证类型: %s", d.AuthType)
	}
	// 端口范围校验
	if d.Port < 0 || d.Port > 65535 {
		return fmt.Errorf("端口超出有效范围: %d", d.Port)
	}
	// 密码明文 → 加密（已加密则保留）
	if d.AuthType == "password" && d.Password != "" && !security.IsEncrypted(d.Password) {
		key, err := security.LoadOrCreateKey(ciDir)
		if err != nil {
			return err
		}
		enc, err := security.EncryptPassword(d.Password, key)
		if err != nil {
			return fmt.Errorf("密码加密失败: %w", err)
		}
		d.Password = enc
	}
	return nil
}

// migrateProjectDeployPasswords 启动时迁移 projects.json 中的明文部署密码为加密存储。
// 确保磁盘上不存留明文密码。已加密（enc: 前缀）的跳过。
func migrateProjectDeployPasswords(ciDir string) {
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	var changed bool
	for i := range cfg.Projects {
		d := cfg.Projects[i].Deploy
		if d == nil {
			continue
		}
		if d.AuthType == "password" && d.Password != "" && !security.IsEncrypted(d.Password) {
			enc, err := security.EncryptPassword(d.Password, key)
			if err == nil {
				cfg.Projects[i].Deploy.Password = enc
				changed = true
			}
		}
	}
	if changed {
		raw, _ := json.MarshalIndent(cfg, "", "  ")
		security.AtomicWriteFile(filepath.Join(ciDir, "projects.json"), raw, 0600)
		log.Printf("🔐 已将 projects.json 中的明文部署密码迁移为加密存储\n")
	}
}
