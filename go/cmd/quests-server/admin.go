package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/httpapi"
	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/config"
	"github.com/quests-inc/quests/go/internal/platform/db"
	"github.com/quests-inc/quests/go/internal/platform/health"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/schedule"
	"github.com/quests-inc/quests/go/internal/store"
)

const adminUsage = `quests-server admin commands (run on the host with access to the database):

  quests-server useradd <username>       create an account (prompts for a password)
  quests-server passwd <username>        change an account's password
  quests-server users                    list accounts (with their two-factor state)
  quests-server secrets-check            does the current encryption key open every sealed value (secrets, LLM keys, 2FA secrets)?
  quests-server 2fa-reset <username>     drop an account's two-factor sign-in and end its sessions (lost phone and recovery codes)
  quests-server token add <name> [user] [--workspace <id>]
                                         mint an API token for a headless client (PostgreSQL: bound to a workspace)
  quests-server workspace ls             list workspaces and their members
  quests-server workspace seed <id>      add/update untouched starter examples
  quests-server workspace seed-remove <id>  remove starter examples
  quests-server workspace add <name> <user>   create a workspace owned by <user>
  quests-server workspace member <id> <user> [owner|member]
                                         add <user> to workspace <id> (default role: member; an existing member gets the new role)
  quests-server workspace restore <id>   undo a deletion within 7 days (ls shows pending purges)
  quests-server token ls                 list active API tokens
  quests-server token rm <id>            revoke an API token
  quests-server runner add <label>       register a device that runs checks (prints a one-time registration token;
                                         PostgreSQL: in workspace $QUESTS_WORKSPACE, default 1)
  quests-server runner ls                list runners and when they were last seen
  quests-server runner rm <label>        revoke a runner and cancel its queued jobs
  quests-server egress status            the policy of outgoing HTTP of scripts (QUESTS_EGRESS_*) and which workspaces may use it
  quests-server egress enable <id>       let workspace <id> use the nodes that go out (it is off for everyone by default)
  quests-server egress disable <id>      take it back
  quests-server egress log [<id> [n]]    the last n outgoing requests (default 50), of one workspace or of all
  quests-server backup-now               run one DB+attachments backup tick now
  quests-server backups                  list the backups (newest first: file, size, where it went, errors)
  quests-server schema                   print the database's schema revision and the one this binary expects
                                         (the server migrates the schema itself when it starts)

Passwords are never taken from argv or env, so they stay out of shell history.
`

// runAdmin handles the bootstrap subcommands. Returns false if args are not an
// admin command, in which case the process starts the server as usual.
func runAdmin(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "schema":
		runSchema()
		return true
	case "useradd", "passwd", "users", "2fa-reset", "secrets-check", "token", "runner", "egress", "backup-now", "backups", "workspace":
	case "help", "-h", "--help":
		fmt.Print(adminUsage)
		return true
	default:
		return false
	}

	config.LoadDotenv(config.Load().Root)
	cfg := config.Load()
	// Admin commands run as the database owner: they are the ones allowed to change what the API role cannot.
	db.SkipAppRole = true
	dbURL, err := cfg.RequireDatabaseURL()
	if err != nil {
		fatal("db: %v", err)
	}
	sqlDB, _, err := db.OpenPostgres(dbURL)
	if err != nil {
		fatal("db: %v", err)
	}
	defer sqlDB.Close()

	if args[0] == "backup-now" {
		runBackupNow(cfg, sqlDB)
		return true
	}
	if args[0] == "backups" {
		runBackups(sqlDB, cfg.DatabaseURL)
		return true
	}

	if args[0] == "egress" {
		runEgressCmd(store.New(sqlDB, cfg.DatabaseURL, nil), args[1:])
		return true
	}

	if args[0] == "runner" {
		runRunnerCmd(store.New(sqlDB, cfg.DatabaseURL, nil), args[1:])
		return true
	}

	box, berr := secretbox.Load(cfg.DataDir)
	if berr != nil {
		fatal("secrets: %v", berr)
	}
	st := &identity.Store{DB: sqlDB, Box: box}
	wst := store.New(sqlDB, cfg.DatabaseURL, box)

	switch args[0] {
	case "useradd":
		if len(args) < 2 {
			fatal("usage: quests-server useradd <username>")
		}
		pw := promptNewPassword()
		u, err := st.CreateUser(args[1], pw)
		if err != nil {
			if errors.Is(err, identity.ErrUserExists) {
				fatal("account %q already exists", args[1])
			}
			fatal("%v", err)
		}
		fmt.Printf("created account %q (id=%d)\n", u.Username, u.ID)
		w, err := httpapi.EnsureUserWorkspace(context.Background(), wst, u.ID, u.Username)
		if err != nil {
			fatal("workspace: %v", err)
		}
		fmt.Printf("workspace %q (id=%d)\n", w.Name, w.ID)

		fmt.Println("auth is now enforced on this instance — restart the server to apply.")
	case "passwd":
		if len(args) < 2 {
			fatal("usage: quests-server passwd <username>")
		}
		pw := promptNewPassword()
		if err := st.SetPassword(args[1], pw); err != nil {
			fatal("%v", err)
		}
		fmt.Println("password updated")
	case "users":
		users, err := st.ListUsers()
		if err != nil {
			fatal("%v", err)
		}
		if len(users) == 0 {
			fmt.Println("no accounts yet — create one with: quests-server useradd <username>")
			return true
		}
		for _, u := range users {
			state := "active"
			if !u.IsActive {
				state = "disabled"
			}
			if on, _ := st.HasMFA(u.ID); on {
				state += "\t2fa"
			}
			fmt.Printf("%d\t%s\t%s\n", u.ID, u.Username, state)
		}
	case "secrets-check":
		counts, err := wst.Execution.SealedHealth(context.Background())
		if err != nil {
			fatal("%v", err)
		}
		bad := 0
		fmt.Printf("%-10s %6s %9s %11s %6s\n", "table", "total", "readable", "unreadable", "plain")
		for _, c := range counts {
			fmt.Printf("%-10s %6d %9d %11d %6d\n", c.Table, c.Total, c.Readable, c.Unreadable, c.Plain)
			bad += c.Unreadable
		}
		if bad > 0 {
			fmt.Println("\nsome values do not open with the current key: it is not the key that sealed them (see docs/security.md)")
			os.Exit(1)
		}
		fmt.Println("\nthe current key opens everything it should")
	case "2fa-reset":
		if len(args) < 2 {
			fatal("usage: quests-server 2fa-reset <username>")
		}
		if err := st.ResetMFA(args[1]); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("two-factor sign-in removed for %q; its sessions are closed\n", args[1])
	case "token":
		runTokenCmd(st, args[1:])
	case "workspace":
		runWorkspaceCmd(st, wst, args[1:])
	}
	return true
}

func runBackups(sqlDB *sql.DB, url string) {
	st := store.New(sqlDB, url, nil)
	rows, err := st.ListBackupLogs(context.Background())
	if err != nil {
		fatal("backups: %v", err)
	}
	if len(rows) == 0 {
		fmt.Println("no backups recorded yet")
		return
	}
	for _, r := range rows {
		where := []string{"local"}
		if r.WebdavUploaded {
			where = append(where, "webdav")
		}
		if r.RemoteUploaded {
			where = append(where, "remote")
		}
		line := fmt.Sprintf("%s\t%s\t%d bytes\t%s", r.CreatedAt, r.Filename, r.SizeBytes, strings.Join(where, "+"))
		if r.AttachmentsFilename != "" {
			line += fmt.Sprintf("\t+ %s (%d bytes)", r.AttachmentsFilename, r.AttachmentsSizeBytes)
		}
		if r.Error != "" {
			line += "\tERROR: " + r.Error
		}
		fmt.Println(line)
	}
}

func runBackupNow(cfg config.Config, sqlDB *sql.DB) {
	st := store.New(sqlDB, cfg.DatabaseURL, nil)
	wd := webdav.New(cfg.WebDAVURL, cfg.WebDAVUser, cfg.WebDAVPass)
	remote := schedule.NewSFTPRemote(schedule.BackupRemote{
		Host: cfg.BackupRemoteHost,
		Dir:  cfg.BackupRemoteDir,
		Key:  cfg.BackupRemoteKey,
		Port: cfg.BackupRemotePort,
	})
	reg := health.New()
	if err := schedule.RunBackupOnce(context.Background(), st, wd, cfg.DataDir, cfg.BackupKeep, remote, reg); err != nil {
		fatal("backup: %v", err)
	}
	snap := reg.Snapshot()
	detail := ""
	if p, ok := snap["backup"].(map[string]any); ok {
		if d, ok := p["detail"].(string); ok {
			detail = d
		}
	}
	if detail == "" {
		detail = "ok"
	}
	fmt.Printf("backup-now: %s\n", detail)
	fmt.Printf("  remote configured=%v host=%q dir=%q\n",
		remote.Configured(), cfg.BackupRemoteHost, cfg.BackupRemoteDir)
}

func runTokenCmd(st *identity.Store, args []string) {
	if len(args) == 0 {
		fatal("usage: quests-server token add|ls|rm")
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			fatal("usage: quests-server token add <name>   (e.g. overlay, telegram, cli)")
		}
		users, err := st.ListUsers()
		if err != nil {
			fatal("%v", err)
		}
		if len(users) == 0 {
			fatal("create an account first: quests-server useradd <username>")
		}
		var wsID int64
		var pos []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--workspace" && i+1 < len(args) {
				n, err := strconv.ParseInt(args[i+1], 10, 64)
				if err != nil || n <= 0 {
					fatal("--workspace needs a workspace id")
				}
				wsID = n
				i++
				continue
			}
			pos = append(pos, args[i])
		}
		owner := users[0]
		if len(pos) >= 2 {
			owner = identity.User{}
			for _, u := range users {
				if u.Username == strings.ToLower(pos[1]) {
					owner = u
				}
			}
			if owner.ID == 0 {
				fatal("no such account: %s", pos[1])
			}
		}
		secret, err := st.CreateAPIToken(owner.ID, pos[0], wsID)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("token for %q (owner %s):\n\n  %s\n\n", pos[0], owner.Username, secret)
		fmt.Println("Shown once — only its hash is stored. Put it in the client's env:")
		fmt.Printf("  QUESTS_API_TOKEN=%s\n", secret)
	case "ls":
		toks, err := st.ListAPITokens()
		if err != nil {
			fatal("%v", err)
		}
		if len(toks) == 0 {
			fmt.Println("no active tokens")
			return
		}
		for _, t := range toks {
			last := "never"
			if t.LastUsedAt != nil {
				last = *t.LastUsedAt
			}
			fmt.Printf("%d\t%s\t%s\tlast_used=%s\n", t.ID, t.Name, t.Username, last)
		}
	case "rm":
		if len(args) < 2 {
			fatal("usage: quests-server token rm <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			fatal("token id must be a number")
		}
		if err := st.RevokeAPIToken(id); err != nil {
			fatal("%v", err)
		}
		fmt.Println("revoked")
	default:
		fatal("unknown token command %q", args[0])
	}
}

// promptNewPassword reads a password twice without echoing it. On a non-TTY
// (CI, docker exec without -t) it falls back to reading one line from stdin.
func promptNewPassword() string {
	if !term.IsTerminal(int(syscall.Stdin)) {
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() {
			fatal("no password on stdin")
		}
		pw := strings.TrimSpace(sc.Text())
		if len([]rune(pw)) < 8 {
			fatal("password must be at least 8 characters")
		}
		return pw
	}
	fmt.Print("New password: ")
	first, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fatal("read password: %v", err)
	}
	fmt.Print("Repeat password: ")
	second, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fatal("read password: %v", err)
	}
	if string(first) != string(second) {
		fatal("passwords do not match")
	}
	if len([]rune(string(first))) < 8 {
		fatal("password must be at least 8 characters")
	}
	return string(first)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}

func runRunnerCmd(st *store.Store, args []string) {
	ctx := context.Background()
	// Runners belong to a workspace: QUESTS_WORKSPACE picks it (default 1).
	ws := int64(identity.DefaultWorkspaceID)
	if raw := strings.TrimSpace(os.Getenv("QUESTS_WORKSPACE")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			fatal("QUESTS_WORKSPACE must be a workspace id")
		}
		ws = n
	}
	ctx = wsctx.With(ctx, ws)

	if len(args) == 0 {
		fatal("usage: quests-server runner add|ls|rm")
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			fatal("usage: quests-server runner add <label>")
		}
		token, err := st.Execution.CreateRunner(ctx, args[1])
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("runner %q created. One-time registration token (valid %d h):\n\n  %s\n\n", args[1], int(execution.RegistrationTTL.Hours()), token)
		fmt.Println("On the device:  quests-runner register --server https://… --token <the token above>")
	case "ls":
		runners, err := st.Execution.ListRunners(ctx)
		if err != nil {
			fatal("%v", err)
		}
		if len(runners) == 0 {
			fmt.Println("no runners — add one with: quests-server runner add <label>")
			return
		}
		now := time.Now()
		for _, r := range runners {
			state := "online"
			switch {
			case r.Revoked:
				state = "revoked"
			case !r.Registered:
				state = "not registered yet"
			case !execution.RunnerOnline(r, now):
				state = "offline"
			}
			seen := "never"
			if r.LastSeenAt != nil {
				seen = r.LastSeenAt.Local().Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%s\t%s\tlast seen %s\t%s\n", r.Label, state, seen, r.Hostname)
		}
	case "rm":
		if len(args) < 2 {
			fatal("usage: quests-server runner rm <label>")
		}
		if err := st.Execution.RevokeRunner(ctx, args[1]); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("runner %q revoked\n", args[1])
	default:
		fatal("usage: quests-server runner add|ls|rm")
	}
}

func runWorkspaceCmd(st *identity.Store, wst *store.Store, args []string) {
	ctx := context.Background()
	userByName := func(name string) identity.User {
		users, err := st.ListUsers()
		if err != nil {
			fatal("%v", err)
		}
		for _, u := range users {
			if u.Username == strings.ToLower(name) {
				return u
			}
		}
		fatal("no such account: %s", name)
		return identity.User{}
	}
	if len(args) == 0 {
		fatal("usage: quests-server workspace ls|add|member")
	}
	switch args[0] {
	case "seed", "seed-remove":
		runStarterCmd(ctx, wst, args)
	case "ls":
		users, err := st.ListUsers()
		if err != nil {
			fatal("%v", err)
		}
		members := map[int64][]string{}
		seen := map[int64]identity.Workspace{}
		for _, u := range users {
			ws, err := wst.Identity.UserWorkspaces(ctx, u.ID)
			if err != nil {
				fatal("%v", err)
			}
			for _, w := range ws {
				if _, ok := seen[w.ID]; !ok {
					seen[w.ID] = w
				}
				members[w.ID] = append(members[w.ID], u.Username)
			}
		}
		ids, err := wst.Identity.WorkspaceIDs(ctx)
		if err != nil {
			fatal("%v", err)
		}
		all, err := wst.Identity.AllWorkspaces(ctx)
		if err != nil {
			fatal("%v", err)
		}
		allByID := map[int64]identity.Workspace{}
		for _, w := range all {
			allByID[w.ID] = w
		}
		for _, id := range ids {
			fmt.Printf("%d\t%s\tmembers: %s\n", id, allByID[id].Name, strings.Join(members[id], ", "))
		}
		dels, err := wst.Identity.DeletedWorkspaces(ctx)
		if err != nil {
			fatal("%v", err)
		}
		for _, d := range dels {
			fmt.Printf("%d\t%s\tDELETED, purge after %s (workspace restore %d)\n", d.ID, d.Name, d.PurgeAt.Local().Format("2006-01-02 15:04"), d.ID)
		}
	case "add":
		if len(args) < 3 {
			fatal("usage: quests-server workspace add <name> <user>")
		}
		u := userByName(args[2])
		w, err := httpapi.ProvisionWorkspace(ctx, wst, u.ID, args[1], identity.DefaultWorkspaceID)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("created workspace %q (id=%d) owned by %s\n", w.Name, w.ID, u.Username)
	case "restore":
		if len(args) < 2 {
			fatal("usage: quests-server workspace restore <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			fatal("workspace id must be a number")
		}
		if err := wst.Identity.RestoreWorkspace(ctx, id); err != nil {
			fatal("%v (already purged, or not deleted?)", err)
		}
		fmt.Printf("workspace %d restored\n", id)
	case "member":
		if len(args) < 3 || len(args) > 4 {
			fatal("usage: quests-server workspace member <id> <user> [owner|member]   (default: member)")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			fatal("workspace id must be a number")
		}
		role := identity.RoleMember
		if len(args) == 4 {
			role = args[3]
		}
		if role != identity.RoleOwner && role != identity.RoleMember {
			fatal("role must be owner or member, got %q", role)
		}
		u := userByName(args[2])
		was, err := wst.Identity.MemberRole(ctx, id, u.ID)
		if err != nil {
			fatal("%v", err)
		}
		if was == role {
			fmt.Printf("%s is already %s of workspace %d\n", u.Username, role, id)
			return
		}
		if err := wst.Identity.AddWorkspaceMember(ctx, id, u.ID, role); err != nil {
			fatal("%v", err)
		}
		if was == "" {
			fmt.Printf("%s is now %s of workspace %d\n", u.Username, role, id)
		} else {
			fmt.Printf("%s: role in workspace %d changed from %s to %s\n", u.Username, id, was, role)
		}
	default:
		fatal("unknown workspace command %q", args[0])
	}
}

// runSchema prints where the database is and where this binary wants it.
func runSchema() {
	config.LoadDotenv(config.Load().Root)
	cfg := config.Load()
	dbURL, err := cfg.RequireDatabaseURL()
	if err != nil {
		fatal("schema: %v", err)
	}
	v, err := db.CurrentRevision(dbURL)
	if err != nil {
		fatal("schema: %v", err)
	}
	if v == "" {
		v = "(none)"
	}
	fmt.Printf("database: %s\nexpected: %s\n", v, db.ExpectedRevision())
}
