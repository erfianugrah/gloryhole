package policy

import (
	"testing"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine() returned nil")
	}
}

func TestAddRule(t *testing.T) {
	e := NewEngine()
	rule := &Rule{
		Name:   "Test Rule",
		Logic:  "true",
		Action: "BLOCK",
	}
	err := e.AddRule(rule)
	if err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}
	if len(e.rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(e.rules))
	}
}
