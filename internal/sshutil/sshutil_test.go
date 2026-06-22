package sshutil

import (
	"path/filepath"
	"testing"
)

func TestKnownHostsPath(t *testing.T) {
	path := KnownHostsPath("/ci-cd")
	expected := filepath.Join("/ci-cd", ".known_hosts")
	if path != expected {
		t.Errorf("期望 %q, 得到 %q", expected, path)
	}
}

func TestKnownHostsPath_Relative(t *testing.T) {
	path := KnownHostsPath(".")
	expected := filepath.Join(".", ".known_hosts")
	if path != expected {
		t.Errorf("期望 %q, 得到 %q", expected, path)
	}
}

func TestKnownHostsPath_Empty(t *testing.T) {
	path := KnownHostsPath("")
	expected := filepath.Join("", ".known_hosts")
	if path != expected {
		t.Errorf("期望 %q, 得到 %q", expected, path)
	}
}
