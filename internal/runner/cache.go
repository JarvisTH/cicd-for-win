// cache.go — 构建缓存，根据源文件修改时间跳过未变更的构建/检查/测试。
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BuildCache 缓存一次构建/检查/测试的结果。
type BuildCache struct {
	Project    string    `json:"project"`
	Action     string    `json:"action"`
	Status     string    `json:"status"`
	Duration   string    `json:"duration"`
	MaxModTime time.Time `json:"max_mod_time"`
}

// cachePath 返回缓存文件路径：ci-cd/cache/{project}/{action}.json
func cachePath(ciDir, project, action string) string {
	return filepath.Join(ciDir, "cache", project, action+".json")
}

// loadCache 从磁盘加载缓存，失败返回 nil。
func loadCache(ciDir, project, action string) *BuildCache {
	data, err := os.ReadFile(cachePath(ciDir, project, action))
	if err != nil {
		return nil
	}
	var c BuildCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

// saveCache 将缓存写入磁盘。
func saveCache(ciDir, project, action string, c *BuildCache) {
	path := cachePath(ciDir, project, action)
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0644)
}

// watchDirs 按项目类型返回需要监视的源文件/目录列表。
func watchDirs(projectType ProjectType) []string {
	switch projectType {
	case ProjectTypeReact, ProjectTypeVue, ProjectTypeNode, ProjectTypeAngular, ProjectTypeNext:
		return []string{"src", "package.json", "tsconfig.json"}
	case ProjectTypeMaven, ProjectTypeMavenMulti:
		return []string{"src", "pom.xml"}
	case ProjectTypeGradle:
		return []string{"src", "build.gradle"}
	case ProjectTypeRust:
		return []string{"src", "Cargo.toml"}
	case ProjectTypeGo:
		return []string{".", "go.mod"} // Go 项目扫描整个目录
	default:
		return []string{"src", "package.json", "pom.xml", "go.mod"}
	}
}

// getLatestModTime 扫描项目源文件，返回最新的修改时间。
// 如果任一监视文件/目录不存在，返回 time.Time{} 使缓存失效。
func getLatestModTime(projectPath string, projectType ProjectType) time.Time {
	var maxTime time.Time
	for _, name := range watchDirs(projectType) {
		path := filepath.Join(projectPath, name)
		info, err := os.Stat(path)
		if err != nil {
			// 监视的文件/目录不存在，无法判断缓存是否有效
			return maxTime
		}
		if info.IsDir() {
			// 递归扫描目录
			filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
				if err == nil && fi.ModTime().After(maxTime) {
					maxTime = fi.ModTime()
				}
				return nil
			})
		} else {
			if info.ModTime().After(maxTime) {
				maxTime = info.ModTime()
			}
		}
	}
	return maxTime
}

// cacheHit 检查缓存是否命中：缓存存在、状态为 pass、且源文件未变更。
// ciDir 为缓存目录基路径；project 为项目名称（用于缓存 key）。
// action 为 "check"/"build"/"test"；projectType 用于确定监视的文件。
func cacheHit(ciDir, projectName, action string, projectType ProjectType, projectPath string) *BuildCache {
	cache := loadCache(ciDir, projectName, action)
	if cache == nil {
		return nil
	}
	if cache.Status != "pass" {
		return nil // 上次失败的缓存不重用
	}
	// 不指定 ciDir 时跳过文件扫描（用于测试）
	if ciDir == "" {
		return cache
	}
	latestMod := getLatestModTime(projectPath, projectType)
	if latestMod.IsZero() || latestMod.After(cache.MaxModTime) {
		return nil // 源文件已变更或无法检测
	}
	return cache
}

// cacheSummary 返回缓存命中的日志消息。
func cacheSummary(cache *BuildCache) string {
	if cache == nil {
		return ""
	}
	return fmt.Sprintf("  ⚡ 源文件未变更，跳过（缓存命中，上次 %s）\n", cache.Duration)
}
