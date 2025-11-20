package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	// This test will be expanded to test loading from a YAML file.
	cfg, err := Load("testdata/config.yml")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
}
