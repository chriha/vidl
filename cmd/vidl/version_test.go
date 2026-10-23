package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	withVersion := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}
	tests := []struct {
		name string
		v    string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"ldflags version wins", "1.2.0", withVersion("v1.1.0"), true, "1.2.0"},
		{"go install version", "dev", withVersion("v1.1.0"), true, "v1.1.0"},
		{"local build", "dev", withVersion("(devel)"), true, "dev"},
		{"empty module version", "dev", withVersion(""), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.v, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallHint(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows", "freebsd"} {
		if installHint(goos) == "" {
			t.Errorf("installHint(%q) is empty", goos)
		}
	}
}
