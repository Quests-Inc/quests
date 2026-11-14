package main

import (
	"encoding/json"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/limits"
	"strconv"
)

func runLimits(args []string) {
	if len(args) == 0 || args[0] == "list" {
		rows, err := limits.List()
		if err != nil {
			fatal("limits: %v", err)
		}
		raw, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(raw))
		return
	}
	if len(args) < 2 {
		fatal("usage: limits list | set <key> <integer> | reset <key>")
	}
	var value *int64
	switch args[0] {
	case "set":
		if len(args) != 3 {
			fatal("usage: limits set <key> <integer>")
		}
		n, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			fatal("value must be an integer in the unit shown by limits list")
		}
		value = &n
	case "reset":
		if len(args) != 2 {
			fatal("usage: limits reset <key>")
		}
	default:
		fatal("unknown limits command")
	}
	if err := limits.Change(args[1], value); err != nil {
		fatal("limits: %v", err)
	}
	rows, err := limits.List()
	if err != nil {
		fatal("limits: %v", err)
	}
	fmt.Printf("%s = %d (%s)\n", args[1], rows[args[1]].Value, rows[args[1]].Source)
}
