package cli

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"five minutes", "5m", 5 * time.Minute, false},
		{"one hour", "1h", time.Hour, false},
		{"thirty seconds", "30s", 30 * time.Second, false},
		{"compound 2h30m", "2h30m", 150 * time.Minute, false},
		{"compound 1h15m30s", "1h15m30s", time.Hour + 15*time.Minute + 30*time.Second, false},
		{"empty string", "", 0, true},
		{"garbage", "abc", 0, true},
		{"negative accepted by Go parser", "-5m", -5 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration(%q) error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBandwidth(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"50mbit", "50mbit", 50, false},
		{"100m", "100m", 100, false},
		{"1000mbit max", "1000mbit", 1000, false},
		{"1mbit min", "1mbit", 1, false},
		{"bare number", "500", 500, false},
		{"uppercase MBIT", "50MBIT", 50, false},
		{"zero mbit", "0mbit", 0, true},
		{"over max", "2000mbit", 0, true},
		{"garbage", "abc", 0, true},
		{"empty", "", 0, true},
		{"negative", "-10mbit", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBandwidth(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseBandwidth(%q) = %d, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseBandwidth(%q) error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseBandwidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"basic", "a,b,c", []string{"a", "b", "c"}},
		{"with spaces", "a, b, c", []string{"a", "b", "c"}},
		{"empty string", "", nil},
		{"trim spaces", "  a  ", []string{"a"}},
		{"single value", "hello", []string{"hello"}},
		{"trailing comma", "a,b,", []string{"a", "b"}},
		{"leading comma", ",a,b", []string{"a", "b"}},
		{"multiple commas", "a,,b", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("splitCSV(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("splitCSV(%q) len = %d, want %d; got %v", tt.input, len(got), len(tt.want), got)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestParsePortList(t *testing.T) {
	tests := []struct {
		name string
		input string
		want  []int
	}{
		{"two ports", "8080,3000", []int{8080, 3000}},
		{"single port", "443", []int{443}},
		{"with spaces", "8080, 3000", []int{8080, 3000}},
		{"empty", "", nil},
		{"non-numeric", "abc", nil},
		{"zero port", "0", nil},
		{"over 65535", "99999", nil},
		{"mixed valid invalid", "8080,abc,3000", []int{8080, 3000}},
		{"port at boundary", "65535", []int{65535}},
		{"port 1", "1", []int{1}},
		{"negative port", "-1", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePortList(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parsePortList(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("parsePortList(%q) len = %d, want %d; got %v", tt.input, len(got), len(tt.want), got)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parsePortList(%q)[%d] = %d, want %d", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestToInt32Slice(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []int32
	}{
		{"basic", []int{1, 2, 3}, []int32{1, 2, 3}},
		{"empty", []int{}, []int32{}},
		{"single", []int{42}, []int32{42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt32Slice(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("toInt32Slice(%v) len = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("toInt32Slice(%v)[%d] = %d, want %d", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestStrPtr(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isNil  bool
		wantVal string
	}{
		{"empty returns nil", "", true, ""},
		{"non-empty returns pointer", "hello", false, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strPtr(tt.input)
			if tt.isNil {
				if got != nil {
					t.Errorf("strPtr(%q) = %v, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Errorf("strPtr(%q) = nil, want %q", tt.input, tt.wantVal)
				return
			}
			if *got != tt.wantVal {
				t.Errorf("strPtr(%q) = %q, want %q", tt.input, *got, tt.wantVal)
			}
		})
	}
}

func TestInt64Ptr(t *testing.T) {
	tests := []struct {
		name    string
		input   int64
		isNil   bool
		wantVal int64
	}{
		{"zero returns nil", 0, true, 0},
		{"positive returns pointer", 42, false, 42},
		{"negative returns pointer", -1, false, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := int64Ptr(tt.input)
			if tt.isNil {
				if got != nil {
					t.Errorf("int64Ptr(%d) = %v, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Errorf("int64Ptr(%d) = nil, want %d", tt.input, tt.wantVal)
				return
			}
			if *got != tt.wantVal {
				t.Errorf("int64Ptr(%d) = %d, want %d", tt.input, *got, tt.wantVal)
			}
		})
	}
}
