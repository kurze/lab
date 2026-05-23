package main

import (
	"reflect"
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

func TestParseMRIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int64
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"single", "42", []int64{42}, false},
		{"multiple", "1,2,5", []int64{1, 2, 5}, false},
		{"spaces", " 1 , 2 , 5 ", []int64{1, 2, 5}, false},
		{"trailing comma", "1,2,", []int64{1, 2}, false},
		{"invalid", "abc", nil, true},
		{"negative", "-1", nil, true},
		{"zero", "0", nil, true},
		{"mixed invalid", "1,abc,3", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMRIDs(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMRIDs(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMRIDs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMRTargets(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []MRTarget
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"id only", "42", []MRTarget{{ID: 42}}, false},
		{"with mode", "42:commits", []MRTarget{{ID: 42, Mode: "commits"}}, false},
		{"mixed", "1,2:both,3:full", []MRTarget{{ID: 1}, {ID: 2, Mode: "both"}, {ID: 3, Mode: "full"}}, false},
		{"invalid mode", "1:bogus", nil, true},
		{"invalid id", "abc:full", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMRTargets(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMRTargets(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMRTargets(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
