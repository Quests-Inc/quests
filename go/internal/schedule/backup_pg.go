package schedule

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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
