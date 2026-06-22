package serve

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"ci-cd/internal/config"
	"ci-cd/internal/security"
)

// ========== 独立服务器管理 ==========

type StandaloneServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	AuthType     string `json:"auth_type"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	Note         string `json:"note,omitempty"`
}

type ServerList struct {
	Servers []StandaloneServer `json:"servers"`
}

func serversFilePath(ciDir string) string {
	return filepath.Join(ciDir, "servers.json")
}

func loadServers(ciDir string) *ServerList {
	path := serversFilePath(ciDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ServerList{Servers: []StandaloneServer{}}
	}
	var list ServerList
	if err := json.Unmarshal(data, &list); err != nil {
		return &ServerList{Servers: []StandaloneServer{}}
	}
	if list.Servers == nil {
		list.Servers = []StandaloneServer{}
	}
	migrateServerPasswords(ciDir, &list)
	return &list
}

func saveServers(ciDir string, list *ServerList) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return security.AtomicWriteFile(serversFilePath(ciDir), data, 0600)
}

func migrateServerPasswords(ciDir string, list *ServerList) {
	var changed bool
	key, keyErr := security.LoadOrCreateKey(ciDir)
	if keyErr != nil {
		return
	}
	for i := range list.Servers {
		s := &list.Servers[i]
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			enc, err := security.EncryptPassword(s.Password, key)
			if err == nil {
				list.Servers[i].Password = enc
				changed = true
			}
		}
	}
	if changed {
		saveServers(ciDir, list)
	}
}

func sanitizeDeploy(deploy map[string]any) map[string]any {
	out := make(map[string]any, len(deploy))
	for k, v := range deploy {
		if k == "password" {
			out[k] = ""
			continue
		}
		out[k] = v
	}
	return out
}

func sanitizeServer(s StandaloneServer) StandaloneServer {
	s.Password = ""
	return s
}

func loadProjectByName(name string) *config.Project {
	ciDir := findCiDir()
	if ciDir == "" {
		return nil
	}
	cfg, err := config.Load(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return nil
	}
	for _, p := range cfg.Projects {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

func resolveDeployConfig(name, source string) *config.DeployConfig {
	if source == "standalone" {
		return loadStandaloneServerDeploy(name)
	}
	proj := loadProjectByName(name)
	if proj != nil {
		return proj.Deploy
	}
	return nil
}

func loadStandaloneServerDeploy(name string) *config.DeployConfig {
	ciDir := findCiDir()
	if ciDir == "" {
		return nil
	}
	list := loadServers(ciDir)
	for _, s := range list.Servers {
		if s.Name == name {
			return &config.DeployConfig{
				Host: s.Host, Port: s.Port, User: s.User,
				AuthType: s.AuthType, IdentityFile: s.IdentityFile, Password: s.Password,
			}
		}
	}
	return nil
}

// ========== HTTP Handlers ==========

func handleRemoteServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		return
	}

	if r.Method == "POST" {
		var s StandaloneServer
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "请求格式错误"})
			return
		}
		if s.Name == "" || s.Host == "" || s.User == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "名称、主机、用户名不能为空"})
			return
		}
		if s.Port == 0 {
			s.Port = 22
		}
		if s.AuthType == "password" && s.Password != "" && !security.IsEncrypted(s.Password) {
			key, err := security.LoadOrCreateKey(ciDir)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": "初始化密钥失败: " + err.Error()})
				return
			}
			enc, err := security.EncryptPassword(s.Password, key)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]string{"error": "密码加密失败: " + err.Error()})
				return
			}
			s.Password = enc
		}
		list := loadServers(ciDir)
		for _, existing := range list.Servers {
			if existing.Name == s.Name {
				json.NewEncoder(w).Encode(map[string]string{"error": "服务器名称已存在"})
				return
			}
		}
		list.Servers = append(list.Servers, s)
		if err := saveServers(ciDir, list); err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
			return
		}
		log.Printf("🖥️ 添加服务器: %s (%s@%s)\n", s.Name, s.User, s.Host)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	list := loadServers(ciDir)
	safe := make([]StandaloneServer, 0, len(list.Servers))
	for _, s := range list.Servers {
		safe = append(safe, sanitizeServer(s))
	}
	json.NewEncoder(w).Encode(map[string]any{"servers": safe})
}

func handleRemoteServerDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "找不到 ci-cd 目录"})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "缺少 name 参数"})
		return
	}
	list := loadServers(ciDir)
	idx := -1
	for i, s := range list.Servers {
		if s.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		json.NewEncoder(w).Encode(map[string]string{"error": "服务器不存在"})
		return
	}
	list.Servers = append(list.Servers[:idx], list.Servers[idx+1:]...)
	if err := saveServers(ciDir, list); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "保存失败: " + err.Error()})
		return
	}
	log.Printf("🗑️ 删除服务器: %s\n", name)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRemoteDeployTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		return
	}

	var items []map[string]any

	if data, err := os.ReadFile(filepath.Join(ciDir, "projects.json")); err == nil {
		var cfg struct {
			Projects []map[string]any `json:"projects"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			for _, p := range cfg.Projects {
				if deploy, ok := p["deploy"].(map[string]any); ok {
					if host, _ := deploy["host"].(string); host != "" {
						name, _ := p["name"].(string)
						pType, _ := p["type"].(string)
						items = append(items, map[string]any{
							"name": "📋 " + name, "source": "project", "ref": name,
							"type": pType, "deploy": sanitizeDeploy(deploy),
						})
					}
				}
			}
		}
	}

	sl := loadServers(ciDir)
	for _, s := range sl.Servers {
		items = append(items, map[string]any{
			"name": "🖥️ " + s.Name, "source": "standalone", "ref": s.Name, "type": "Server",
			"deploy": map[string]any{
				"host": s.Host, "port": s.Port, "user": s.User,
				"auth_type": s.AuthType, "identity_file": s.IdentityFile, "password": "",
			},
		})
	}

	if items == nil {
		items = []map[string]any{}
	}
	json.NewEncoder(w).Encode(map[string]any{"servers": items})
}
