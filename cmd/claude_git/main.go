package main

import (
	"os"

	"github.com/angerer/claude_git/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
