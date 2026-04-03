package dns

import (
	"testing"
)

func TestClientACL_EmptyIsOpen(t *testing.T) {
	acl := NewClientACL(nil)
	if !acl.IsOpen() {
		t.Fatal("empty ACL should be open")
	}
	if !acl.IsAllowed("1.2.3.4") {
		t.Fatal("empty ACL should allow any IP")
	}
}

func TestClientACL_SingleIP(t *testing.T) {
	acl := NewClientACL([]string{"192.168.1.100"})
	if acl.IsOpen() {
		t.Fatal("non-empty ACL should not be open")
	}
	if !acl.IsAllowed("192.168.1.100") {
		t.Fatal("should allow listed IP")
	}
	if acl.IsAllowed("192.168.1.101") {
		t.Fatal("should deny unlisted IP")
	}
}

func TestClientACL_CIDR(t *testing.T) {
	acl := NewClientACL([]string{"10.0.0.0/8"})
	if !acl.IsAllowed("10.1.2.3") {
		t.Fatal("should allow IP in CIDR range")
	}
	if !acl.IsAllowed("10.255.255.255") {
		t.Fatal("should allow IP at end of CIDR range")
	}
	if acl.IsAllowed("11.0.0.1") {
		t.Fatal("should deny IP outside CIDR range")
	}
}

func TestClientACL_MixedEntries(t *testing.T) {
	acl := NewClientACL([]string{
		"192.168.1.0/24",
		"10.0.0.1",
		"172.16.0.0/12",
	})

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"192.168.1.50", true},
		{"192.168.2.1", false},
		{"10.0.0.1", true},
		{"10.0.0.2", false},
		{"172.16.5.10", true},
		{"172.32.0.1", false},
		{"8.8.8.8", false},
	}

	for _, tc := range tests {
		got := acl.IsAllowed(tc.ip)
		if got != tc.allowed {
			t.Errorf("IsAllowed(%q) = %v, want %v", tc.ip, got, tc.allowed)
		}
	}
}

func TestClientACL_IPv6(t *testing.T) {
	acl := NewClientACL([]string{"::1", "fd00::/8"})
	if !acl.IsAllowed("::1") {
		t.Fatal("should allow IPv6 loopback")
	}
	if !acl.IsAllowed("fd00::1") {
		t.Fatal("should allow IP in fd00::/8")
	}
	if acl.IsAllowed("2001:db8::1") {
		t.Fatal("should deny IP outside allowed ranges")
	}
}

func TestClientACL_InvalidIP(t *testing.T) {
	acl := NewClientACL([]string{"10.0.0.0/8"})
	if acl.IsAllowed("not-an-ip") {
		t.Fatal("should deny invalid IP string")
	}
	if acl.IsAllowed("") {
		t.Fatal("should deny empty string")
	}
}

func TestClientACL_Update(t *testing.T) {
	acl := NewClientACL([]string{"10.0.0.1"})
	if !acl.IsAllowed("10.0.0.1") {
		t.Fatal("should allow before update")
	}
	if acl.IsAllowed("10.0.0.2") {
		t.Fatal("should deny before update")
	}

	// Update to a new range
	acl.Update([]string{"10.0.0.0/24"})
	if !acl.IsAllowed("10.0.0.2") {
		t.Fatal("should allow after update")
	}
	if acl.IsAllowed("10.0.1.1") {
		t.Fatal("should deny outside updated range")
	}
}

func TestClientACL_UpdateToEmpty(t *testing.T) {
	acl := NewClientACL([]string{"10.0.0.1"})
	if acl.IsOpen() {
		t.Fatal("should not be open")
	}

	acl.Update(nil)
	if !acl.IsOpen() {
		t.Fatal("should be open after update to empty")
	}
	if !acl.IsAllowed("1.2.3.4") {
		t.Fatal("should allow any IP when open")
	}
}

func TestClientACL_SkipsInvalidEntries(t *testing.T) {
	acl := NewClientACL([]string{"not-valid", "10.0.0.1", "also/bad"})
	if !acl.IsAllowed("10.0.0.1") {
		t.Fatal("should allow valid entry despite invalid siblings")
	}
	if acl.IsAllowed("10.0.0.2") {
		t.Fatal("should deny unlisted IP")
	}
}

func BenchmarkClientACL_IsAllowed(b *testing.B) {
	acl := NewClientACL([]string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fd00::/8",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acl.IsAllowed("192.168.1.50")
	}
}
