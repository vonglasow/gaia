package main

import (
	"fmt"
	"os"

	"gaia/kernel"
	"gaia/plugins"
)

func main() {
	k := kernel.NewKernel()
	if err := plugins.RegisterAll(k); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := k.Execute(os.Args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
