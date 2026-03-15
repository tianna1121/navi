package main

import (
	"os"

	"github.com/tianna1121/navi/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
