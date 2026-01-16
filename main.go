package main

import (
	"os"

	"github.com/grovetools/skills/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
