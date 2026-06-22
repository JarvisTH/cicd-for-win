package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ci-cd/internal/config"
	"ci-cd/internal/runner"
)

func stepStatusDir(ciDir string) string {
	return filepath.Join(ciDir, "status")
}

func saveStepStatus(ciDir string, result runner.Result) {
	if result.Project == "" || result.Action == "" {
		return
	}
	dir := filepath.Join(stepStatusDir(ciDir), result.Project)
	os.MkdirAll(dir, config.DirPermDefault)
	path := filepath.Join(dir, result.Action+".json")
	data, _ := json.Marshal(result)
	os.WriteFile(path, data, config.FilePermDefault)
}

type stepStatusFile struct {
	Status   string `json:"status"`
	ErrorLog string `json:"error_log,omitempty"`
}

func loadStepStatuses(ciDir string) map[string]map[string]stepStatusFile {
	base := stepStatusDir(ciDir)
	result := make(map[string]map[string]stepStatusFile)
	entries, err := os.ReadDir(base)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		project := entry.Name()
		projectDir := filepath.Join(base, project)
		stepFiles, _ := os.ReadDir(projectDir)
		steps := make(map[string]stepStatusFile)
		for _, sf := range stepFiles {
			if sf.IsDir() || filepath.Ext(sf.Name()) != ".json" {
				continue
			}
			action := strings.TrimSuffix(sf.Name(), ".json")
			data, err := os.ReadFile(filepath.Join(projectDir, sf.Name()))
			if err != nil {
				continue
			}
			var s stepStatusFile
			if json.Unmarshal(data, &s) == nil {
				steps[action] = s
			}
		}
		if len(steps) > 0 {
			result[project] = steps
		}
	}
	return result
}

func stepStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"error": "找不到 ci-cd 目录"})
		return
	}
	statuses := loadStepStatuses(ciDir)
	json.NewEncoder(w).Encode(map[string]any{"statuses": statuses})
}

func stepStatusClearHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	project := r.URL.Query().Get("project")
	ciDir := findCiDir()
	if ciDir == "" {
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "message": "找不到 ci-cd 目录"})
		return
	}
	if project != "" {
		dir := filepath.Join(stepStatusDir(ciDir), project)
		os.RemoveAll(dir)
	} else {
		dir := stepStatusDir(ciDir)
		os.RemoveAll(dir)
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
