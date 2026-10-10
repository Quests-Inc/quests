package schedule

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// pgDump writes a custom-format (pg_restore-able) dump of the database at url.
// Credentials travel in PG* environment variables, not on the command line.
func pgDump(ctx context.Context, url, outPath string) error {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	env := append(os.Environ(),
		"PGHOST="+cfg.Host,
		"PGPORT="+strconv.Itoa(int(cfg.Port)),
		"PGUSER="+cfg.User,
		"PGDATABASE="+cfg.Database,
	)
	if cfg.Password != "" {
		env = append(env, "PGPASSWORD="+cfg.Password)
	}
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--file", outPath)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outPath)
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PreMigrationBackup dumps the database to <dataDir>/backups/pre-migrate-<from>-to-<to>-<time>.dump before
// a schema migration touches it, and keeps the newest keep such dumps. A missing pg_dump is not an
// error (a bare binary on a host without PostgreSQL tools): it is reported through warn and the
// migration goes on; a pg_dump that runs and fails stops it.
func PreMigrationBackup(ctx context.Context, url, dataDir, from, to string, keep int, warn func(string, ...any)) (string, error) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		warn("pre-migration backup skipped: pg_dump is not installed")
		return "", nil
	}
	dir := filepath.Join(dataDir, backupSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("pre-migrate-%s-to-%s-%s.dump", from, to, time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := dumpDatabase(ctx, url, path); err != nil {
		return "", err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "pre-migrate-*.dump"))
	// the name ends with -YYYYMMDD-HHMMSS.dump; the revisions before it do not order by time
	stamp := func(p string) string {
		b := strings.TrimSuffix(filepath.Base(p), ".dump")
		return b[max(0, len(b)-15):]
	}
	sort.Slice(old, func(i, j int) bool { return stamp(old[i]) < stamp(old[j]) })
	for len(old) > keep {
		_ = os.Remove(old[0])
		old = old[1:]
	}
	return path, nil
}
