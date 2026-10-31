//go:build !windows

package ipc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fallbackSocket points the temp dir at a fresh short dir and returns the
// fallback socket path for a long config dir, without creating anything.
func fallbackSocket(t *testing.T) string {
	t.Helper()
	t.Setenv("TMPDIR", shortDir(t))
	return SocketPath(filepath.Join("/", strings.Repeat("d", 120), "vidl"))
}

func TestFallbackDirRejected(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		uid   func() int
	}{
		{"mode too open", func(t *testing.T, dir string) {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"owned by another user", func(t *testing.T, dir string) {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}, func() int { return os.Getuid() + 1 }},
		{"symlink to a private dir", func(t *testing.T, dir string) {
			target := filepath.Join(filepath.Dir(dir), "elsewhere")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, dir); err != nil {
				t.Fatal(err)
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fallbackSocket(t)
			dir := filepath.Dir(path)
			tt.setup(t, dir)
			if tt.uid != nil {
				old := currentUID
				currentUID = tt.uid
				t.Cleanup(func() { currentUID = old })
			}

			if s, err := Listen(path, func(Request) (int, error) { return 0, nil }); err == nil {
				s.Close()
				t.Error("Listen accepted an unsafe fallback dir")
			}
			if _, err := Send(path, Request{Add: []string{"https://example.com/a"}}, time.Second); err == nil || errors.Is(err, ErrNoServer) {
				t.Errorf("Send = %v, want an unsafe-dir error", err)
			}
		})
	}
}

func TestFallbackDirAccepted(t *testing.T) {
	path := fallbackSocket(t)
	s, err := Listen(path, func(Request) (int, error) { return 1, nil })
	if err != nil {
		t.Fatalf("Listen on fresh fallback dir: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	go s.Serve()
	if n, err := Send(path, Request{Add: []string{"https://example.com/a"}}, time.Second); err != nil || n != 1 {
		t.Errorf("Send = %d, %v, want 1, nil", n, err)
	}
}

func TestConfigDirModeNotChecked(t *testing.T) {
	dir := shortDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Listen(filepath.Join(dir, SocketName), func(Request) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("Listen in a 0755 config dir: %v", err)
	}
	s.Close()
}
