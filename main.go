package main

import (
	"fmt"
	"gaia/commands"
	"os"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
