package ytdlp

import (
	"testing"
	"time"
)

func TestParseClipTime(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "0", want: 0},
		{in: "45", want: 45 * time.Second},
		{in: "90", want: 90 * time.Second},
		{in: "1:30", want: 90 * time.Second},
		{in: "01:05", want: 65 * time.Second},
		{in: "1:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
		{in: "0:00:07", want: 7 * time.Second},
		{in: "12.5", want: 12500 * time.Millisecond},
		{in: "1:30.25", want: 90250 * time.Millisecond},
		{in: " 1:30 ", want: 90 * time.Second},
		{in: "", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "-5", wantErr: true},
		{in: "1:60", wantErr: true},
		{in: "1:60:00", wantErr: true},
		{in: "1:2:3:4", wantErr: true},
		{in: "1:", wantErr: true},
		{in: ":30", wantErr: true},
		{in: "1.5:30", wantErr: true},
		{in: "1:+30", wantErr: true},
		{in: "1e3", wantErr: true},
		{in: "inf", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseClipTime(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseClipTime(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseClipTime(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestParseClip(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		want       Clip
		wantErr    bool
	}{
		{name: "both empty is no clip", want: Clip{}},
		{name: "both ends", start: "1:30", end: "2:00", want: Clip{Start: 90 * time.Second, End: 120 * time.Second}},
		{name: "open start", end: "45", want: Clip{End: 45 * time.Second}},
		{name: "open end", start: "1:00", want: Clip{Start: time.Minute}},
		{name: "end equals start", start: "30", end: "0:30", wantErr: true},
		{name: "end before start", start: "2:00", end: "1:00", wantErr: true},
		{name: "explicit zero end", start: "0", end: "0", wantErr: true},
		{name: "bad start", start: "x", end: "10", wantErr: true},
		{name: "bad end", start: "10", end: "1:99", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClip(tt.start, tt.end)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseClip(%q, %q) = %+v, want error", tt.start, tt.end, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseClip(%q, %q) = %+v, %v; want %+v", tt.start, tt.end, got, err, tt.want)
			}
		})
	}
}
