// quests-runner executes checks for a Quests server on this device. It only
// makes outgoing HTTPS requests: it long-polls the server for jobs, runs each
// command, reports progress and delivers the result.
//
//	quests-runner register --server https://quests.example --token <registration token>
//	quests-runner run [--concurrency 1]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// version is stamped at build time (-ldflags "-X main.version=…").
var version = "dev"

const usage = `quests-runner — runs Quests checks on this device

  quests-runner register --server URL --token TOKEN   exchange a one-time registration token
  quests-runner run [--concurrency N]                 poll the server and run jobs
  quests-runner version

Options (both commands):
  --config FILE   config path (default: $XDG_CONFIG_HOME/quests-runner/config.json)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "--version":
		fmt.Println(version)
	case "register":
		cmdRegister(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func cmdRegister(args []string) {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	server := fs.String("server", os.Getenv("QUESTS_RUNNER_SERVER"), "server base URL")
	token := fs.String("token", "", "one-time registration token")
	cfgPath := fs.String("config", "", "config file")
	_ = fs.Parse(args)
	if *server == "" || *token == "" {
		fatalf("--server and --token are required")
	}
	host, _ := os.Hostname()
	cl := newClient(*server, "")
	label, secret, err := cl.register(context.Background(), *token, host, version)
	if err != nil {
		fatalf("register: %v", err)
	}
	path := configPath(*cfgPath)
	if err := saveConfig(path, config{Server: *server, Token: secret, Label: label}); err != nil {
		fatalf("save config: %v", err)
	}
	fmt.Printf("registered as %q; config written to %s\nstart it with: quests-runner run\n", label, path)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file")
	conc := fs.Int("concurrency", 1, "jobs run at the same time")
	_ = fs.Parse(args)
	cfg, err := loadConfig(configPath(*cfgPath))
	if err != nil {
		fatalf("%v (run `quests-runner register` first)", err)
	}
	if *conc < 1 {
		*conc = 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := newAgent(cfg, newClient(cfg.Server, cfg.Token), spoolDir())
	log.Printf("quests-runner %s: %q polling %s (concurrency %d)", version, cfg.Label, cfg.Server, *conc)
	if err := a.run(ctx, *conc); err != nil && ctx.Err() == nil {
		fatalf("%v", err)
	}
}
