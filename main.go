package main

import (
	"gaia/commands"
	"gaia/config"
	"log"
)

func main() {
	err := config.InitConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Initialize the API client within the commands package
	if err := commands.InitAPIClient(); err != nil {
		log.Fatalf("Failed to initialize API client: %v", err)
	}

	err = commands.Execute()
	if err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
}
