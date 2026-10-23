package ytdlp

import (
	"testing"
	"time"
)

func TestStaleDays(t *testing.T) {
	now := time.Date(2026, 10, 23, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		version string
		days    int
		stale   bool
		wantErr bool
	}{
		{"release", "2026.08.01", 83, true, false},
		{"nightly", "2026.08.01.232853", 83, true, false},
		{"trailing newline", "2026.10.20\n", 3, false, false},
		{"released today", "2026.10.23", 0, false, false},
		{"exactly 60 days", "2026.08.24", 60, false, false},
		{"61 days", "2026.08.23", 61, true, false},
		{"garbage", "master-abc123", 0, false, true},
		{"git build", "2026.10.20-dev", 0, false, true},
		{"empty", "", 0, false, true},
		{"impossible date", "2026.13.40", 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, stale, err := StaleDays(tt.version, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if days != tt.days || stale != tt.stale {
				t.Errorf("got %d days stale=%v, want %d stale=%v", days, stale, tt.days, tt.stale)
			}
		})
	}
}
