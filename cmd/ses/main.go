package main

import (
	"os"

	commands "github.com/timae/rel.ai/cmd"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
