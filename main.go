package main

import (
	"fmt"
	"os"

	"github.com/transpose-project/transpose/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
