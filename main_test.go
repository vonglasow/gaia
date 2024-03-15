package main

import "testing"

func TestConvert(t *testing.T) {
	config, err := loadConfig(false)
	if err != nil {
		t.Logf("error loading config: %v", err)
		t.Fail()
	}

	expectedConfig := Config{
		OLLAMA_MODEL:    "openhermes2.5-mistral",
		OLLAMA_BASE_URL: "http://localhost:11434",
	}

	if config.OLLAMA_MODEL != expectedConfig.OLLAMA_MODEL {
		t.Logf("incorrect OLLAMA_MODEL, got: %s, want: %s", config.OLLAMA_MODEL, expectedConfig.OLLAMA_MODEL)
		t.Fail()
	}

	if config.OLLAMA_BASE_URL != expectedConfig.OLLAMA_BASE_URL {
		t.Logf("incorrect OLLAMA_BASE_URL, got: %s, want: %s", config.OLLAMA_BASE_URL, expectedConfig.OLLAMA_BASE_URL)
		t.Fail()
	}
}
