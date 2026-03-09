package nftables

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// --- SetupConfig.Validate() ---

func TestSetupConfig_Validate_MissingVMId(t *testing.T) {
	c := &SetupConfig{Policy: PolicyAllowAll}
	if err := c.Validate(); !errors.Is(err, ErrMissingVMId) {
		t.Errorf("got %v, want ErrMissingVMId", err)
	}
}

func TestSetupConfig_Validate_MissingPolicy(t *testing.T) {
	c := &SetupConfig{VMId: "vm-1"}
	if err := c.Validate(); !errors.Is(err, ErrMissingPolicy) {
		t.Errorf("got %v, want ErrMissingPolicy", err)
	}
}

func TestSetupConfig_Validate_InvalidPolicy(t *testing.T) {
	c := &SetupConfig{VMId: "vm-1", Policy: "block"}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid policy")
	}
	if !strings.Contains(err.Error(), "invalid policy") {
		t.Errorf("got %v, want error containing 'invalid policy'", err)
	}
}

func TestSetupConfig_Validate_ValidPolicies(t *testing.T) {
	for _, policy := range []string{PolicyAllowAll, PolicyDenyAll, PolicyCustom} {
		t.Run(policy, func(t *testing.T) {
			c := &SetupConfig{VMId: "vm-1", Policy: policy}
			if err := c.Validate(); err != nil {
				t.Errorf("Validate() = %v for valid policy %q", err, policy)
			}
		})
	}
}

func TestSetupConfig_Validate_InvalidGuestIP(t *testing.T) {
	tests := []struct {
		name    string
		guestIP string
	}{
		{"garbage", "not-an-ip"},
		{"too many octets", "1.2.3.4.5"},
		{"empty segments", "1..2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &SetupConfig{VMId: "vm-1", Policy: PolicyAllowAll, GuestIP: tt.guestIP}
			err := c.Validate()
			if err == nil {
				t.Fatal("expected error for invalid guestIp")
			}
			if !strings.Contains(err.Error(), "guestIp") {
				t.Errorf("got %v, want error mentioning guestIp", err)
			}
		})
	}
}

func TestSetupConfig_Validate_InvalidHostIP(t *testing.T) {
	tests := []struct {
		name   string
		hostIP string
	}{
		{"garbage", "not-an-ip"},
		{"too many octets", "1.2.3.4.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &SetupConfig{VMId: "vm-1", Policy: PolicyAllowAll, HostIP: tt.hostIP}
			err := c.Validate()
			if err == nil {
				t.Fatal("expected error for invalid hostIp")
			}
			if !strings.Contains(err.Error(), "hostIp") {
				t.Errorf("got %v, want error mentioning hostIp", err)
			}
		})
	}
}

func TestSetupConfig_Validate_IPv6Rejected(t *testing.T) {
	tests := []struct {
		name  string
		field string
		cfg   SetupConfig
	}{
		{
			"guestIp IPv6",
			"guestIp",
			SetupConfig{VMId: "vm-1", Policy: PolicyAllowAll, GuestIP: "::1"},
		},
		{
			"hostIp IPv6",
			"hostIp",
			SetupConfig{VMId: "vm-1", Policy: PolicyAllowAll, HostIP: "fe80::1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for IPv6 %s", tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("got %v, want error mentioning %s", err, tt.field)
			}
			if !strings.Contains(err.Error(), "IPv4") {
				t.Errorf("got %v, want error mentioning IPv4", err)
			}
		})
	}
}

func TestSetupConfig_Validate_PublishedPort_HostPortZero(t *testing.T) {
	c := &SetupConfig{
		VMId:   "vm-1",
		Policy: PolicyAllowAll,
		PublishedPorts: []PublishedPort{
			{HostPort: 0, GuestPort: 80, Protocol: "tcp"},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for hostPort=0")
	}
	if !strings.Contains(err.Error(), "hostPort") {
		t.Errorf("got %v, want error mentioning hostPort", err)
	}
}

func TestSetupConfig_Validate_PublishedPort_GuestPortZero(t *testing.T) {
	c := &SetupConfig{
		VMId:   "vm-1",
		Policy: PolicyAllowAll,
		PublishedPorts: []PublishedPort{
			{HostPort: 8080, GuestPort: 0, Protocol: "tcp"},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for guestPort=0")
	}
	if !strings.Contains(err.Error(), "guestPort") {
		t.Errorf("got %v, want error mentioning guestPort", err)
	}
}

func TestSetupConfig_Validate_PublishedPort_InvalidProtocol(t *testing.T) {
	c := &SetupConfig{
		VMId:   "vm-1",
		Policy: PolicyAllowAll,
		PublishedPorts: []PublishedPort{
			{HostPort: 8080, GuestPort: 80, Protocol: "sctp"},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Errorf("got %v, want error mentioning protocol", err)
	}
}

func TestSetupConfig_Validate_PublishedPort_ValidProtocols(t *testing.T) {
	for _, proto := range []string{"tcp", "udp", ""} {
		t.Run("protocol="+proto, func(t *testing.T) {
			c := &SetupConfig{
				VMId:   "vm-1",
				Policy: PolicyAllowAll,
				PublishedPorts: []PublishedPort{
					{HostPort: 8080, GuestPort: 80, Protocol: proto},
				},
			}
			if err := c.Validate(); err != nil {
				t.Errorf("Validate() = %v for valid protocol %q", err, proto)
			}
		})
	}
}

func TestSetupConfig_Validate_InvalidAllowedCIDR(t *testing.T) {
	c := &SetupConfig{
		VMId:         "vm-1",
		Policy:       PolicyCustom,
		AllowedCIDRs: []string{"not-a-cidr"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid CIDR in allowedCidrs")
	}
	if !strings.Contains(err.Error(), "allowedCidrs") {
		t.Errorf("got %v, want error mentioning allowedCidrs", err)
	}
}

func TestSetupConfig_Validate_InvalidDeniedCIDR(t *testing.T) {
	c := &SetupConfig{
		VMId:        "vm-1",
		Policy:      PolicyCustom,
		DeniedCIDRs: []string{"bad/cidr"},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for invalid CIDR in deniedCidrs")
	}
	if !strings.Contains(err.Error(), "deniedCidrs") {
		t.Errorf("got %v, want error mentioning deniedCidrs", err)
	}
}

func TestSetupConfig_Validate_InvalidDNSResolver(t *testing.T) {
	tests := []struct {
		name     string
		resolver string
	}{
		{"garbage", "not-an-ip"},
		{"IPv6", "::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &SetupConfig{
				VMId:         "vm-1",
				Policy:       PolicyAllowAll,
				DNSResolvers: []string{tt.resolver},
			}
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for DNS resolver %q", tt.resolver)
			}
			if !strings.Contains(err.Error(), "dnsResolvers") {
				t.Errorf("got %v, want error mentioning dnsResolvers", err)
			}
		})
	}
}

func TestSetupConfig_Validate_ValidComplete(t *testing.T) {
	c := &SetupConfig{
		VMId:             "vm-42",
		Slot:             3,
		Policy:           PolicyCustom,
		TapDevice:        "tap0",
		HostIP:           "198.19.0.1",
		GuestIP:          "198.19.0.2",
		VethHost:         "veth0h",
		VethGuest:        "veth0g",
		NetNSName:        "vmsan-vm-42",
		DefaultInterface: "eth0",
		PublishedPorts: []PublishedPort{
			{HostPort: 8080, GuestPort: 80, Protocol: "tcp"},
			{HostPort: 8443, GuestPort: 443, Protocol: "udp"},
		},
		AllowedCIDRs: []string{"10.0.0.0/8", "192.168.0.0/16"},
		DeniedCIDRs:  []string{"172.16.0.0/12"},
		DNSResolvers: []string{"1.1.1.1", "8.8.8.8"},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v for valid complete config", err)
	}
}

// --- TeardownConfig.Validate() ---

func TestTeardownConfig_Validate_MissingVMId(t *testing.T) {
	c := &TeardownConfig{}
	if err := c.Validate(); !errors.Is(err, ErrMissingVMId) {
		t.Errorf("got %v, want ErrMissingVMId", err)
	}
}

func TestTeardownConfig_Validate_Valid(t *testing.T) {
	c := &TeardownConfig{VMId: "vm-1", NetNSName: "vmsan-vm-1"}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v for valid config", err)
	}
}

// --- VerifyConfig.Validate() ---

func TestVerifyConfig_Validate_MissingVMId(t *testing.T) {
	c := &VerifyConfig{}
	if err := c.Validate(); !errors.Is(err, ErrMissingVMId) {
		t.Errorf("got %v, want ErrMissingVMId", err)
	}
}

func TestVerifyConfig_Validate_Valid(t *testing.T) {
	c := &VerifyConfig{VMId: "vm-1", NetNSName: "vmsan-vm-1"}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v for valid config", err)
	}
}

// --- CleanupConfig.Validate() ---

func TestCleanupConfig_Validate_MissingVMId(t *testing.T) {
	c := &CleanupConfig{}
	if err := c.Validate(); !errors.Is(err, ErrMissingVMId) {
		t.Errorf("got %v, want ErrMissingVMId", err)
	}
}

func TestCleanupConfig_Validate_Valid(t *testing.T) {
	c := &CleanupConfig{
		VMId:      "vm-1",
		TapDevice: "tap0",
		VethHost:  "veth0h",
		VethGuest: "veth0g",
		NetNSName: "vmsan-vm-1",
		HostIP:    "198.19.0.1",
		GuestIP:   "198.19.0.2",
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v for valid config", err)
	}
}

// --- JSON serialization round-trip ---

func TestSetupConfig_JSONRoundTrip(t *testing.T) {
	original := SetupConfig{
		VMId:             "vm-42",
		Slot:             3,
		Policy:           PolicyCustom,
		TapDevice:        "tap0",
		HostIP:           "198.19.0.1",
		GuestIP:          "198.19.0.2",
		VethHost:         "veth0h",
		VethGuest:        "veth0g",
		NetNSName:        "vmsan-vm-42",
		DefaultInterface: "eth0",
		PublishedPorts: []PublishedPort{
			{HostPort: 8080, GuestIP: "198.19.0.2", GuestPort: 80, Protocol: "tcp"},
			{HostPort: 8443, GuestIP: "198.19.0.2", GuestPort: 443, Protocol: "udp"},
		},
		AllowedCIDRs: []string{"10.0.0.0/8"},
		DeniedCIDRs:  []string{"172.16.0.0/12"},
		SkipDNAT:     true,
		DNSResolvers: []string{"1.1.1.1", "8.8.8.8"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SetupConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify scalar fields
	if decoded.VMId != original.VMId {
		t.Errorf("VMId = %q, want %q", decoded.VMId, original.VMId)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot = %d, want %d", decoded.Slot, original.Slot)
	}
	if decoded.Policy != original.Policy {
		t.Errorf("Policy = %q, want %q", decoded.Policy, original.Policy)
	}
	if decoded.TapDevice != original.TapDevice {
		t.Errorf("TapDevice = %q, want %q", decoded.TapDevice, original.TapDevice)
	}
	if decoded.HostIP != original.HostIP {
		t.Errorf("HostIP = %q, want %q", decoded.HostIP, original.HostIP)
	}
	if decoded.GuestIP != original.GuestIP {
		t.Errorf("GuestIP = %q, want %q", decoded.GuestIP, original.GuestIP)
	}
	if decoded.VethHost != original.VethHost {
		t.Errorf("VethHost = %q, want %q", decoded.VethHost, original.VethHost)
	}
	if decoded.VethGuest != original.VethGuest {
		t.Errorf("VethGuest = %q, want %q", decoded.VethGuest, original.VethGuest)
	}
	if decoded.NetNSName != original.NetNSName {
		t.Errorf("NetNSName = %q, want %q", decoded.NetNSName, original.NetNSName)
	}
	if decoded.DefaultInterface != original.DefaultInterface {
		t.Errorf("DefaultInterface = %q, want %q", decoded.DefaultInterface, original.DefaultInterface)
	}
	if decoded.SkipDNAT != original.SkipDNAT {
		t.Errorf("SkipDNAT = %v, want %v", decoded.SkipDNAT, original.SkipDNAT)
	}

	// Verify slices
	if len(decoded.PublishedPorts) != len(original.PublishedPorts) {
		t.Fatalf("PublishedPorts length = %d, want %d", len(decoded.PublishedPorts), len(original.PublishedPorts))
	}
	for i, pp := range decoded.PublishedPorts {
		orig := original.PublishedPorts[i]
		if pp.HostPort != orig.HostPort || pp.GuestPort != orig.GuestPort ||
			pp.GuestIP != orig.GuestIP || pp.Protocol != orig.Protocol {
			t.Errorf("PublishedPorts[%d] = %+v, want %+v", i, pp, orig)
		}
	}
	if len(decoded.AllowedCIDRs) != len(original.AllowedCIDRs) {
		t.Fatalf("AllowedCIDRs length = %d, want %d", len(decoded.AllowedCIDRs), len(original.AllowedCIDRs))
	}
	for i, cidr := range decoded.AllowedCIDRs {
		if cidr != original.AllowedCIDRs[i] {
			t.Errorf("AllowedCIDRs[%d] = %q, want %q", i, cidr, original.AllowedCIDRs[i])
		}
	}
	if len(decoded.DeniedCIDRs) != len(original.DeniedCIDRs) {
		t.Fatalf("DeniedCIDRs length = %d, want %d", len(decoded.DeniedCIDRs), len(original.DeniedCIDRs))
	}
	for i, cidr := range decoded.DeniedCIDRs {
		if cidr != original.DeniedCIDRs[i] {
			t.Errorf("DeniedCIDRs[%d] = %q, want %q", i, cidr, original.DeniedCIDRs[i])
		}
	}
	if len(decoded.DNSResolvers) != len(original.DNSResolvers) {
		t.Fatalf("DNSResolvers length = %d, want %d", len(decoded.DNSResolvers), len(original.DNSResolvers))
	}
	for i, r := range decoded.DNSResolvers {
		if r != original.DNSResolvers[i] {
			t.Errorf("DNSResolvers[%d] = %q, want %q", i, r, original.DNSResolvers[i])
		}
	}
}

func TestSetupConfig_JSONRoundTrip_OptionalFieldsOmitted(t *testing.T) {
	// Minimal config with only required fields
	original := SetupConfig{
		VMId:   "vm-1",
		Policy: PolicyAllowAll,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify omitempty fields are absent from JSON
	jsonStr := string(data)
	for _, field := range []string{"defaultInterface", "publishedPorts", "allowedCidrs", "deniedCidrs", "skipDnat", "dnsResolvers"} {
		if strings.Contains(jsonStr, field) {
			t.Errorf("JSON should omit empty field %q, got: %s", field, jsonStr)
		}
	}

	var decoded SetupConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.VMId != original.VMId {
		t.Errorf("VMId = %q, want %q", decoded.VMId, original.VMId)
	}
	if decoded.Policy != original.Policy {
		t.Errorf("Policy = %q, want %q", decoded.Policy, original.Policy)
	}
	if decoded.PublishedPorts != nil {
		t.Errorf("PublishedPorts should be nil, got %v", decoded.PublishedPorts)
	}
	if decoded.AllowedCIDRs != nil {
		t.Errorf("AllowedCIDRs should be nil, got %v", decoded.AllowedCIDRs)
	}
	if decoded.DeniedCIDRs != nil {
		t.Errorf("DeniedCIDRs should be nil, got %v", decoded.DeniedCIDRs)
	}
	if decoded.DNSResolvers != nil {
		t.Errorf("DNSResolvers should be nil, got %v", decoded.DNSResolvers)
	}
	if decoded.SkipDNAT {
		t.Error("SkipDNAT should be false")
	}
	if decoded.DefaultInterface != "" {
		t.Errorf("DefaultInterface should be empty, got %q", decoded.DefaultInterface)
	}
}

func TestNftResult_JSON(t *testing.T) {
	// Success result omits error and code
	ok := NftResult{OK: true}
	data, err := json.Marshal(ok)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	jsonStr := string(data)
	if strings.Contains(jsonStr, "error") {
		t.Errorf("success result should omit error field, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "code") {
		t.Errorf("success result should omit code field, got: %s", jsonStr)
	}

	// Error result includes error and code
	fail := NftResult{OK: false, Error: "table not found", Code: "NFT_ERR_MISSING_TABLE"}
	data, err = json.Marshal(fail)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded NftResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.OK != false {
		t.Error("OK should be false")
	}
	if decoded.Error != "table not found" {
		t.Errorf("Error = %q, want %q", decoded.Error, "table not found")
	}
	if decoded.Code != "NFT_ERR_MISSING_TABLE" {
		t.Errorf("Code = %q, want %q", decoded.Code, "NFT_ERR_MISSING_TABLE")
	}
}

func TestVerifyResult_JSON(t *testing.T) {
	vr := VerifyResult{
		NftResult:   NftResult{OK: true},
		TableExists: true,
		ChainCount:  5,
	}
	data, err := json.Marshal(vr)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded VerifyResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.OK {
		t.Error("OK should be true")
	}
	if !decoded.TableExists {
		t.Error("TableExists should be true")
	}
	if decoded.ChainCount != 5 {
		t.Errorf("ChainCount = %d, want 5", decoded.ChainCount)
	}
}
