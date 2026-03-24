package config

import (
	"testing"
)

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"both empty", "", "", 0},
		{"exact match", "abc", "abc", 0},
		{"one deletion", "abc", "ab", 1},
		{"one substitution", "node22", "nod22", 1},
		{"completely different", "abc", "xyz", 3},
		{"a empty", "", "abc", 3},
		{"b empty", "abc", "", 3},
		{"one insertion", "ab", "abc", 1},
		{"transposition", "ab", "ba", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Levenshtein(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFindClosestMatch(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		candidates []string
		want       string
	}{
		{
			name:       "close match nod22",
			input:      "nod22",
			candidates: ValidRuntimes,
			want:       "node22",
		},
		{
			name:       "exact match",
			input:      "python3.13",
			candidates: ValidRuntimes,
			want:       "python3.13",
		},
		{
			name:       "too far away",
			input:      "xyz",
			candidates: ValidRuntimes,
			want:       "",
		},
		{
			name:       "close match bas",
			input:      "bas",
			candidates: ValidRuntimes,
			want:       "base",
		},
		{
			name:       "empty candidates",
			input:      "node22",
			candidates: []string{},
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindClosestMatch(tt.input, tt.candidates)
			if got != tt.want {
				t.Errorf("FindClosestMatch(%q, ...) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
