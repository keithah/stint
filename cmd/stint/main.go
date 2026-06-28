package main

import (
	"fmt"
	"os"

	"github.com/keithah/stint/internal/stintcli"
)

func main() {
	if err := stintcli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
