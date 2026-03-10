package compat

import "testing"

func TestContainsAny_ExactMatch(t *testing.T) {
	line := "-A FORWARD -s 10.0.0.1/32 -j ACCEPT"
	if !containsAny(line, []string{"10.0.0.1"}) {
		t.Error("should match exact IP")
	}
}

func TestContainsAny_NoPartialIPMatch(t *testing.T) {
	line := "-A FORWARD -s 10.0.0.10/32 -j ACCEPT"
	if containsAny(line, []string{"10.0.0.1"}) {
		t.Error("10.0.0.1 should NOT match 10.0.0.10")
	}
}

func TestContainsAny_NoPartialIPMatch_Hundred(t *testing.T) {
	line := "-A FORWARD -d 10.0.0.100 -j DROP"
	if containsAny(line, []string{"10.0.0.1"}) {
		t.Error("10.0.0.1 should NOT match 10.0.0.100")
	}
}

func TestContainsAny_MatchWithSlash(t *testing.T) {
	line := "-A FORWARD -s 10.0.0.1/32 -j ACCEPT"
	if !containsAny(line, []string{"10.0.0.1"}) {
		t.Error("should match IP followed by /32")
	}
}

func TestContainsAny_DeviceName(t *testing.T) {
	line := "-A FORWARD -i tap0 -o veth0h -j ACCEPT"
	if !containsAny(line, []string{"tap0"}) {
		t.Error("should match device name")
	}
}

func TestContainsAny_NoMatch(t *testing.T) {
	line := "-A FORWARD -s 192.168.1.1/32 -j ACCEPT"
	if containsAny(line, []string{"10.0.0.1", "tap0"}) {
		t.Error("should not match unrelated rule")
	}
}

func TestContainsAny_MultiplePatterns(t *testing.T) {
	line := "-A FORWARD -i tap0 -d 10.0.0.1 -j ACCEPT"
	if !containsAny(line, []string{"tap0", "10.0.0.1"}) {
		t.Error("should match when any pattern matches")
	}
}
