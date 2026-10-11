package main

import (
	"os"

	"github.com/quests-inc/quests/go/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
