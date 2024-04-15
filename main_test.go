package main

import (
	"gaia/config"
	"testing"
)

func TestInitConfig(t *testing.T) {
	err := config.InitConfig()
	if err != nil {
		t.Fatalf("Error initializing config: %v", err)
	}
}
