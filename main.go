package main

import (
	"os"

	"github.com/woyin/skills-manager/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
