package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/store"
)

// runEgressCmd shows and changes who may send HTTP from a script. The policy itself is in the environment of the server
// (QUESTS_EGRESS_*, config.env of the deployment); here is only the per-workspace grant and the log.
func runEgressCmd(st *store.Store, args []string) {
	ctx := context.Background()
	if len(args) == 0 {
		fatal("usage: quests-server egress status|enable <id>|disable <id>|log [<id> [n]]")
	}
	wsID := func(i int) int64 {
		if len(args) <= i {
			fatal("usage: quests-server egress %s <workspace id>", args[0])
		}
		n, err := strconv.ParseInt(args[i], 10, 64)
		if err != nil || n <= 0 {
			fatal("workspace id must be a number")
		}
		return n
	}
	switch args[0] {
	case "status":
		p, err := egress.PolicyFromEnv(os.Getenv)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("mode: %s\n", p.Mode)
		if len(p.Allow) > 0 {
			fmt.Printf("hosts: %s\n", strings.Join(p.Allow, ", "))
		}
		for _, n := range p.AllowNets {
			fmt.Printf("internal exception: %s\n", n)
		}
		fmt.Printf("limits: %d B per answer, %s, %d per hour per workspace, %d at once\n", p.MaxBody, p.Timeout, p.RatePerHour, p.Concurrency)
		grants, err := st.Execution.EgressGrants(ctx)
		if err != nil {
			fatal("%v", err)
		}
		if len(grants) == 0 {
			fmt.Println("no workspace may go out (egress enable <id>)")
		}
		for id, on := range grants {
			state := "off"
			if on {
				state = "ON"
			}
			fmt.Printf("workspace %d: %s\n", id, state)
		}
	case "enable", "disable":
		id := wsID(1)
		if err := st.Execution.SetEgressGrant(ctx, id, args[0] == "enable"); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("workspace %d: outgoing HTTP %sd\n", id, args[0])
	case "log":
		var id int64
		n := 50
		if len(args) > 1 {
			id = wsID(1)
		}
		if len(args) > 2 {
			v, err := strconv.Atoi(args[2])
			if err != nil || v < 1 {
				fatal("usage: quests-server egress log [<workspace id> [n]]")
			}
			n = v
		}
		lines, err := st.Execution.RecentEgress(ctx, id, n)
		if err != nil {
			fatal("%v", err)
		}
		for _, l := range lines {
			fmt.Printf("%s  ws=%d script=%d  %s %s  %d  %dB %dms  %s\n", l.At.Local().Format("2006-01-02 15:04:05"), l.Workspace, l.ScriptID, l.Method, l.Host, l.Status, l.Bytes, l.Millis, l.Result)
		}
		if len(lines) == 0 {
			fmt.Println("no requests logged")
		}
	default:
		fatal("usage: quests-server egress status|enable <id>|disable <id>|log [<id> [n]]")
	}
}
