package firewall

import "testing"

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		patterns []string
		want     bool
	}{
		{
			name:     "exact device match",
			line:     "-A FORWARD -i veth-h-0 -o eth0 -s 198.19.0.0/30 -j ACCEPT",
			patterns: []string{"veth-h-0"},
			want:     true,
		},
		{
			name:     "exact IP match",
			line:     "-A FORWARD -d 198.19.0.2 -j ACCEPT",
			patterns: []string{"198.19.0.2"},
			want:     true,
		},
		{
			name:     "normalized subnet matches network address",
			line:     "-A POSTROUTING -s 198.19.0.0/30 -o eth0 -j MASQUERADE",
			patterns: []string{"198.19.0.0"},
			want:     true,
		},
		{
			name:     "guest IP does not match normalized subnet",
			line:     "-A POSTROUTING -s 198.19.0.0/30 -o eth0 -j MASQUERADE",
			patterns: []string{"198.19.0.2"},
			want:     false,
		},
		{
			name:     "no partial IP match",
			line:     "-A FORWARD -d 198.19.0.10 -j ACCEPT",
			patterns: []string{"198.19.0.1"},
			want:     false,
		},
		{
			name:     "no match",
			line:     "-A FORWARD -j DOCKER-USER",
			patterns: []string{"veth-h-0", "198.19.0.2"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAny(tt.line, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesAny(%q, %v) = %v, want %v", tt.line, tt.patterns, got, tt.want)
			}
		})
	}
}
