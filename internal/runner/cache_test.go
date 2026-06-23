package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===================== cachePath =====================

func TestCachePath(t *testing.T) {
	path := cachePath("/ci-cd", "my-project", "check")
	expected := filepath.Join("/ci-cd", "cache", "my-project", "check.json")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

// ===================== saveCache / loadCache =====================

func TestSaveAndLoadCache(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	c := &BuildCache{
		Project:    "test-proj",
		Action:     "check",
		Status:     "pass",
		Duration:   "1.5s",
		MaxModTime: now,
	}

	saveCache(dir, "test-proj", "check", c)

	loaded := loadCache(dir, "test-proj", "check")
	if loaded == nil {
		t.Fatal("loaded cache should not be nil")
	}
	if loaded.Project != "test-proj" {
		t.Errorf("expected project test-proj, got %s", loaded.Project)
	}
	if loaded.Status != "pass" {
		t.Errorf("expected status pass, got %s", loaded.Status)
	}
	if loaded.Duration != "1.5s" {
		t.Errorf("expected duration 1.5s, got %s", loaded.Duration)
	}
}

func TestLoadCache_FileNotFound(t *testing.T) {
	cache := loadCache(t.TempDir(), "nonexistent", "check")
	if cache != nil {
		t.Error("cache for nonexistent project should be nil")
	}
}

func TestLoadCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cache", "bad-proj"), 0755)
	os.WriteFile(filepath.Join(dir, "cache", "bad-proj", "check.json"), []byte("invalid json"), 0644)

	cache := loadCache(dir, "bad-proj", "check")
	if cache != nil {
		t.Error("invalid JSON cache should return nil")
	}
}

// ===================== getLatestModTime =====================

func TestGetLatestModTime_React(t *testing.T) {
	dir := t.TempDir()
	// Create src dir with a file
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	srcFile := filepath.Join(srcDir, "App.tsx")
	os.WriteFile(srcFile, []byte("content"), 0644)
	// Wait 1ms to ensure file time is different
	time.Sleep(time.Millisecond)

	pkgFile := filepath.Join(dir, "package.json")
	os.WriteFile(pkgFile, []byte("{}"), 0644)

	latest := getLatestModTime(dir, ProjectTypeReact)
	if latest.IsZero() {
		t.Fatal("latest mod time should not be zero")
	}
}

func TestGetLatestModTime_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	latest := getLatestModTime(dir, ProjectTypeReact)
	if !latest.IsZero() {
		t.Error("empty dir should return zero time")
	}
}

// ===================== cacheHit =====================

func TestCacheHit_Valid(t *testing.T) {
	dir := t.TempDir()

	// Create source files FIRST
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "App.tsx"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	// THEN set MaxModTime to a time AFTER the files were created (1 hour in the future)
	future := time.Now().Add(1 * time.Hour)
	saveCache(dir, "test-proj", "check", &BuildCache{
		Project: "test-proj", Action: "check", Status: "pass", Duration: "1.0s",
		MaxModTime: future,
	})

	// Cache should hit (source files are older than future)
	cache := cacheHit(dir, "test-proj", "check", ProjectTypeReact, dir)
	if cache == nil {
		t.Fatal("cache should hit when source files are older than cached time")
	}
	if cache.Status != "pass" {
		t.Errorf("expected pass, got %s", cache.Status)
	}
}

func TestCacheHit_SourceChanged(t *testing.T) {
	dir := t.TempDir()

	// Create a src file with recent timestamp
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	srcFile := filepath.Join(srcDir, "App.tsx")
	os.WriteFile(srcFile, []byte("content"), 0644)
	pkgFile := filepath.Join(dir, "package.json")
	os.WriteFile(pkgFile, []byte("{}"), 0644)

	// Save cache with MaxModTime in the PAST (before source file was created)
	past := time.Now().Add(-1 * time.Hour)
	saveCache(dir, "test-proj", "check", &BuildCache{
		Project: "test-proj", Action: "check", Status: "pass", Duration: "1.0s",
		MaxModTime: past,
	})

	// Cache should miss (source files are newer than cached time)
	cache := cacheHit(dir, "test-proj", "check", ProjectTypeReact, dir)
	if cache != nil {
		t.Error("cache should miss when source files are newer than cached time")
	}
}

func TestCacheHit_FailedStatus(t *testing.T) {
	dir := t.TempDir()

	saveCache(dir, "test-proj", "build", &BuildCache{
		Project: "test-proj", Action: "build", Status: "fail", Duration: "0.5s",
	})

	// Cache should miss (last status was fail)
	cache := cacheHit(dir, "test-proj", "build", ProjectTypeReact, dir)
	if cache != nil {
		t.Error("cache should miss when last status was fail")
	}
}

func TestCacheHit_NoCache(t *testing.T) {
	cache := cacheHit(t.TempDir(), "unknown", "check", ProjectTypeReact, t.TempDir())
	if cache != nil {
		t.Error("no cache should return nil")
	}
}

// ===================== cacheSummary =====================

func TestCacheSummary(t *testing.T) {
	c := &BuildCache{Duration: "2.0s"}
	msg := cacheSummary(c)
	if msg == "" {
		t.Error("cache summary should not be empty")
	}
}

func TestCacheSummary_Nil(t *testing.T) {
	msg := cacheSummary(nil)
	if msg != "" {
		t.Error("nil cache should return empty string")
	}
}

// ===================== watchDirs =====================

func TestWatchDirs_React(t *testing.T) {
	dirs := watchDirs(ProjectTypeReact)
	if len(dirs) == 0 {
		t.Error("React should have watch dirs")
	}
	hasSrc := false
	for _, d := range dirs {
		if d == "src" {
			hasSrc = true
		}
	}
	if !hasSrc {
		t.Error("React should watch src/")
	}
}

func TestWatchDirs_Maven(t *testing.T) {
	dirs := watchDirs(ProjectTypeMaven)
	hasSrc := false
	hasPom := false
	for _, d := range dirs {
		if d == "src" {
			hasSrc = true
		}
		if d == "pom.xml" {
			hasPom = true
		}
	}
	if !hasSrc || !hasPom {
		t.Error("Maven should watch src/ and pom.xml")
	}
}

func TestWatchDirs_Unknown(t *testing.T) {
	dirs := watchDirs(ProjectTypeUnknown)
	if len(dirs) == 0 {
		t.Error("Unknown type should have fallback watch dirs")
	}
}
