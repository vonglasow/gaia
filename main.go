// Command gaia is a CLI for working with local models.
package main

import (
	"fmt"
	"os"

	"gaia/kernel"
	"gaia/plugins"
	"gaia/plugins/shared"
)

func main() {
	k := kernel.NewKernel()
	if err := plugins.RegisterAll(k); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := k.Execute(os.Args[1:]); err != nil {
		// ErrReported was already shown; printing it again would add a bare "reported".
		if !shared.WasReported(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
