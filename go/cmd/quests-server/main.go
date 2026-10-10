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

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/clamav"
	"github.com/valirum/quests/go/internal/config"
	"github.com/valirum/quests/go/internal/db"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/health"
	"github.com/valirum/quests/go/internal/httpapi"
	"github.com/valirum/quests/go/internal/metrics"
	"github.com/valirum/quests/go/internal/schedule"
	"github.com/valirum/quests/go/internal/secretbox"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/webdav"
)

func main() {
	if runAdmin(os.Args[1:]) {
		return
	}

	config.LoadDotenv(config.Load().Root)
	cfg := config.Load()

	dbURL, err := cfg.RequireDatabaseURL()
	if err != nil {
		log.Fatalf("db: %v", err)
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
	authStore := &auth.Store{DB: sqlDB, Box: secrets}
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

	internalToken, _, err := auth.NewToken()
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	hub := events.New()
	st := &store.Store{DB: sqlDB, DatabaseURL: cfg.DatabaseURL, Box: secrets}
	if n, err := st.SealLegacySecrets(context.Background()); err != nil {
		log.Printf("secrets: sealing existing values: %v", err)
	} else if n > 0 {
		log.Printf("secrets: sealed %d stored value(s)", n)
	}
	if counts, err := st.SealedHealth(context.Background()); err != nil {
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
		Store:         st,
		Auth:          authStore,
		AuthRequired:  authRequired,
		SecureCookies: cfg.SecureCookies,
		InternalToken: internalToken,
		Health:        reg,
		Metrics:       metrics.New(sqlDB, st, hub, reg),
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
	go schedule.RunMaintenanceLoop(ctx, st, hub, windows, checks)
	go srv.RunProbes(ctx)
	go schedule.RunWorkspacePurgeLoop(ctx, st, srv.WebDAV)
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
