package main

import "testing"

func TestFirstline(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello\nworld", "hello"},
		{"single line", "single line"},
		{"", ""},
		{"\nleading newline", "\nleading newline"},
		{"trailing\n", "trailing"},
	}
	for _, tt := range tests {
		if got := firstline(tt.in); got != tt.want {
			t.Errorf("firstline(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
