package dns

import (
	"testing"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.Blocklist == nil {
		t.Error("Blocklist should not be nil")
	}
	if h.Whitelist == nil {
		t.Error("Whitelist should not be nil")
	}
	if h.Overrides == nil {
		t.Error("Overrides should not be nil")
	}
	if h.CNAMEOverrides == nil {
		t.Error("CNAMEOverrides should not be nil")
	}
}
