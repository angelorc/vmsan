package mesh

import (
	"testing"
)

func TestValidateACLEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   ACLEntry
		wantErr bool
	}{
		{
			name: "valid tcp entry",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "10.90.0.2",
				DstPort: 5432,
				Proto:   "tcp",
			},
			wantErr: false,
		},
		{
			name: "valid udp entry",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "10.90.0.2",
				DstPort: 6379,
				Proto:   "udp",
			},
			wantErr: false,
		},
		{
			name: "invalid source IP",
			entry: ACLEntry{
				SrcIP:   "not-an-ip",
				DstIP:   "10.90.0.2",
				DstPort: 5432,
				Proto:   "tcp",
			},
			wantErr: true,
		},
		{
			name: "invalid destination IP",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "invalid",
				DstPort: 5432,
				Proto:   "tcp",
			},
			wantErr: true,
		},
		{
			name: "zero port",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "10.90.0.2",
				DstPort: 0,
				Proto:   "tcp",
			},
			wantErr: true,
		},
		{
			name: "invalid protocol",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "10.90.0.2",
				DstPort: 5432,
				Proto:   "icmp",
			},
			wantErr: true,
		},
		{
			name: "empty protocol",
			entry: ACLEntry{
				SrcIP:   "10.90.0.1",
				DstIP:   "10.90.0.2",
				DstPort: 5432,
				Proto:   "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateACLEntry(tt.entry)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateACLEntry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMeshFirewallAllowMeshValidation(t *testing.T) {
	fw := NewMeshFirewall(nil)

	// AllowMesh with empty entries should succeed (no-op).
	if err := fw.AllowMesh(nil); err != nil {
		t.Errorf("AllowMesh(nil) = %v, want nil", err)
	}
	if err := fw.AllowMesh([]ACLEntry{}); err != nil {
		t.Errorf("AllowMesh([]) = %v, want nil", err)
	}

	// AllowMesh with invalid entry should fail validation before calling nft.
	err := fw.AllowMesh([]ACLEntry{{
		SrcIP:   "bad",
		DstIP:   "10.90.0.2",
		DstPort: 5432,
		Proto:   "tcp",
	}})
	if err == nil {
		t.Error("AllowMesh with invalid entry should return error")
	}
}

func TestNewMeshFirewallNilLogger(t *testing.T) {
	fw := NewMeshFirewall(nil)
	if fw == nil {
		t.Fatal("NewMeshFirewall(nil) returned nil")
	}
	if fw.logger == nil {
		t.Error("logger should be set to default when nil is passed")
	}
}

func TestACLEntryFields(t *testing.T) {
	entry := ACLEntry{
		SrcIP:   "10.90.0.1",
		DstIP:   "10.90.0.2",
		DstPort: 5432,
		Proto:   "tcp",
	}

	if entry.SrcIP != "10.90.0.1" {
		t.Errorf("SrcIP = %q, want %q", entry.SrcIP, "10.90.0.1")
	}
	if entry.DstIP != "10.90.0.2" {
		t.Errorf("DstIP = %q, want %q", entry.DstIP, "10.90.0.2")
	}
	if entry.DstPort != 5432 {
		t.Errorf("DstPort = %d, want %d", entry.DstPort, 5432)
	}
	if entry.Proto != "tcp" {
		t.Errorf("Proto = %q, want %q", entry.Proto, "tcp")
	}
}
