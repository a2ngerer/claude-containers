package main

import (
	"os"

	"github.com/angerer/claude_git/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	root.SetArgs(cli.DispatchArgs(os.Args[1:]))
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
