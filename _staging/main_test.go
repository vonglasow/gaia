package main

import (
	"os"
	"path/filepath"
	"testing"

	"gaia/config"
	"gaia/kernel"
	"gaia/plugins"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestInitConfig(t *testing.T) {
	viper.Reset()
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	if err != nil {
		t.Fatalf("Error initializing config: %v", err)
	}
}

func TestMain_Execute(t *testing.T) {
	viper.Reset()
	// Test that main function doesn't panic
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(config.CfgFile, []byte("plugins:\n  enabled: []\n  disabled: []\n"), 0o644))

	// Set args to help command to avoid actual execution
	os.Args = []string{"gaia", "--help"}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("main() panicked: %v", r)
		}
	}()

	// We can't actually call main() as it would exit, but we can test Execute
	k := kernel.NewKernel()
	if err := plugins.RegisterAll(k); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	err := k.Execute(os.Args[1:])
	if err != nil {
		t.Logf("Execute returned error (expected for help): %v", err)
	}
}
