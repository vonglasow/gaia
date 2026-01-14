package main

import (
	"gaia/commands"
	"gaia/config"
	"os"
	"path/filepath"
	"testing"
)

func TestInitConfig(t *testing.T) {
	err := config.InitConfig()
	if err != nil {
		t.Fatalf("Error initializing config: %v", err)
	}
}

func TestMain_Execute(t *testing.T) {
	// Test that main function doesn't panic
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")

	// Set args to help command to avoid actual execution
	os.Args = []string{"gaia", "--help"}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("main() panicked: %v", r)
		}
	}()

	// We can't actually call main() as it would exit, but we can test Execute
	err := commands.Execute()
	if err != nil {
		t.Logf("Execute returned error (expected for help): %v", err)
	}
}
