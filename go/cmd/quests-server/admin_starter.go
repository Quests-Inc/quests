package main

import (
	"context"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
	"strconv"
)

func runStarterCmd(ctx context.Context, wst *store.Store, args []string) {
	if len(args) != 2 {
		fatal("usage: quests-server workspace %s <id>", args[0])
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || id < 1 {
		fatal("invalid workspace id")
	}
	wctx := wsctx.With(ctx, id)
	if args[0] == "seed" {
		report, err := wst.SeedStarter(wctx)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("starter v%d: added=%d updated=%d preserved=%d deleted=%d\n", report.Version, report.Added, report.Updated, report.Preserved, report.Deleted)
	} else {
		n, err := wst.RemoveStarter(wctx)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("trashed=%d\n", n)
	}
}
