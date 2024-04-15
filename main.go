package main

import (
	"fmt"
	"gaia/commands"
	"gaia/config"
	"os"
)

var ()

func main() {
	err := config.InitConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = commands.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
