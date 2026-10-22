package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quests-inc/quests/go/internal/httpapi"
	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/clamav"
	"github.com/quests-inc/quests/go/internal/platform/config"
	"github.com/quests-inc/quests/go/internal/platform/db"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/health"
	"github.com/quests-inc/quests/go/internal/platform/metrics"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/tokens"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/schedule"
	"github.com/quests-inc/quests/go/internal/store"
)

// version is the release this binary was built from (set with -ldflags "-X main.version=1.2.3");
// "dev" for a local build.
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}
	if runAdmin(os.Args[1:]) {
		return
	}

	config.LoadDotenv(config.Load().Root)
	cfg := config.Load()

	dbURL, err := cfg.RequireDatabaseURL()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	// The schema is brought up to date here, before anything serves: starting the bare binary is
	// enough. An existing database is dumped first when there is something to apply.
	migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), 10*time.Minute)
	applied, err := db.MigratePostgresOpts(migrateCtx, dbURL, db.MigrateOptions{
		Logf: log.Printf,
		BeforeApply: func(ctx context.Context, from string, todo []db.Migration) error {
			path, err := schedule.PreMigrationBackup(ctx, dbURL, cfg.DataDir, from, todo[len(todo)-1].Revision, 5, log.Printf)
			if path != "" {
				log.Printf("db: pre-migration backup %s", path)
			}
			return err
		},
	})
	cancelMigrate()
	if err != nil {
		log.Fatalf("db: migrate: %v", err)
	}
	if len(applied) > 0 {
		log.Printf("db: schema migrated to %s", applied[len(applied)-1])
	}
	sqlDB, schema, err := db.OpenPostgres(dbURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer sqlDB.Close()
	if schema.State == "warn" {
		log.Printf("db: warning: %s", schema.Detail)
	}

	secrets, err := secretbox.Load(cfg.DataDir)
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}
	authStore := &identity.Store{DB: sqlDB, Box: secrets}
	userCount, err := authStore.CountUsers()
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	authRequired := userCount > 0
	switch cfg.AuthMode {
	case "on":
		authRequired = true
	case "off":
		authRequired = false
	}
	if authRequired && userCount == 0 {
		log.Fatal("QUESTS_AUTH=on but no accounts exist; create one: quests-server useradd <username>")
	}
	// Refuse to expose an unauthenticated instance beyond loopback — the whole
	// point of accounts is that this cannot happen by forgetting a step.
	if !authRequired && !cfg.IsLoopbackBind() {
		log.Fatalf("refusing to bind %s with authentication disabled.\n"+
			"Create an account first:  quests-server useradd <username>\n"+
			"(or set QUESTS_AUTH=off deliberately for a trusted private network)", cfg.Addr())
	}

	internalToken, _, err := tokens.New()
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	hub := events.New()
	st := store.New(sqlDB, cfg.DatabaseURL, secrets)
	if n, err := st.Execution.SealLegacySecrets(context.Background()); err != nil {
		log.Printf("secrets: sealing existing values: %v", err)
	} else if n > 0 {
		log.Printf("secrets: sealed %d stored value(s)", n)
	}
	if counts, err := st.Execution.SealedHealth(context.Background()); err != nil {
		log.Printf("secrets: checking sealed values: %v", err)
	} else {
		for _, c := range counts {
			if c.Unreadable > 0 {
				log.Printf("WARNING secrets: %d of %d sealed value(s) in %s cannot be opened with the current key "+
					"(was data/secret.key or QUESTS_SECRET_KEY replaced?) — see `quests-server secrets-check`", c.Unreadable, c.Total, c.Table)
			}
		}
	}
	reg := health.New()
	reg.SetProbe("db", schema.State, schema.Detail)
	srv := &httpapi.Server{
		Version:       version,
		Store:         st,
		Auth:          authStore,
		AuthRequired:  authRequired,
		SecureCookies: cfg.SecureCookies,
		InternalToken: internalToken,
		Health:        reg,
		Metrics:       metrics.New(sqlDB, statsSource{st}, hub, reg),
		MetricsToken:  cfg.MetricsToken,
		Hub:           hub,
		CORS:          cfg.CORS,
		DataDir:       cfg.DataDir,
		Root:          cfg.Root,
		SelfBase:      fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),

		WebDAV:         webdav.New(cfg.WebDAVURL, cfg.WebDAVUser, cfg.WebDAVPass),
		ClamAV:         clamav.New(cfg.ClamAVAddr),
		MaxUploadBytes: cfg.MaxUploadBytes,
		WorkspaceQuota: cfg.WorkspaceQuotaBytes,
		Secrets:        secrets,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	windows := schedule.NewWindowNotifier()
	checks := schedule.NewCheckRunner(st, hub)
	srv.RunnerDone = checks.JobFinished
	srv.StartEmbedded = checks.StartEmbedded
	hub.OnPublish(func(p events.Payload) { checks.DispatchHooks(p) })
	go schedule.RunMaintenanceLoop(ctx, st, hub, windows, checks)
	go srv.RunProbes(ctx)
	go schedule.RunWorkspacePurgeLoop(ctx, st, srv.WebDAV)
	go srv.RunTrashPurgeLoop(ctx)
	go schedule.RunBackupLoop(ctx, st, srv.WebDAV, cfg.DataDir, cfg.BackupInterval, cfg.BackupKeep, schedule.BackupRemote{
		Host: cfg.BackupRemoteHost,
		Dir:  cfg.BackupRemoteDir,
		Key:  cfg.BackupRemoteKey,
		Port: cfg.BackupRemotePort,
	}, srv.Health)

	hub.Publish("startup", events.PublishOpts{
		System: true,
		Title:  "Quests",
		Detail: "server ready",
		Toast:  false,
		Source: "system",
		Sound:  strPtr(""),
	})

	addr := cfg.Addr()
	httpSrv := &http.Server{
		Addr: addr, Handler: srv.Handler(),
		// No Read/WriteTimeout: archives and attachments stream for minutes, and the event socket is long-lived.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	authNote := "auth=on"
	if !authRequired {
		authNote = "auth=OFF (loopback only)"
	}
	log.Printf("quests-server (go) listening on http://%s  %s", addr, authNote)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func strPtr(s string) *string { return &s }
