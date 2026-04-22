package main

import (
	"os"

	commands "github.com/timae/ses/cmd"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
