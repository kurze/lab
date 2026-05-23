package main

import (
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, ""},
		{"now", time.Now(), "now"},
		{"30 seconds", time.Now().Add(-30 * time.Second), "now"},
		{"5 minutes", time.Now().Add(-5 * time.Minute), "5m"},
		{"2 hours", time.Now().Add(-2 * time.Hour), "2h"},
		{"3 days", time.Now().Add(-3 * 24 * time.Hour), "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAge(tt.t)
			if got != tt.want {
				t.Errorf("formatAge() = %q, want %q", got, tt.want)
			}
		})
	}
}
