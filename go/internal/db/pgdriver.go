package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/valirum/quests/go/internal/wsctx"
)

// The store layer is written in SQLite dialect (`?` placeholders,
// Result.LastInsertId). Rather than touch ~150 call sites, the PostgreSQL
// connection is wrapped so that
//   - `?` placeholders become `$1, $2, …` (quotes respected), and
//   - a plain `INSERT INTO t …` gets `RETURNING id` appended and its
//     LastInsertId is served from the returned row.
//
// Everything else must be portable SQL (ON CONFLICT, LIKE, …).
//
// Workspaces: the workspace on the statement's context (wsctx) is written to
// the session setting app.workspace_id before the statement runs, which is what
// the row-level-security policies compare against. No workspace on the context
// means the empty setting, i.e. no rows visible (fail closed).

// noIDTables are insert targets without a serial `id` column.
var noIDTables = map[string]bool{
	"quest_tag":        true,
	"template_tag":     true,
	"alembic_version":  true,
	"workspace_member": true,
	"user_llm":         true,
	"user_totp":        true,
	"note_stat":        true,
}

var insertRe = regexp.MustCompile(`(?is)^\s*INSERT\s+INTO\s+"?([a-z_][a-z0-9_]*)"?`)

// rebind rewrites `?` placeholders to `$n`, skipping '…' literals and "…" identifiers.
func rebind(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	var quote byte
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			b.WriteByte(c)
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// insertReturningID reports whether q is a bare INSERT whose id we should return.
func insertReturningID(q string) bool {
	m := insertRe.FindStringSubmatch(q)
	if m == nil || noIDTables[strings.ToLower(m[1])] {
		return false
	}
	return !strings.Contains(strings.ToUpper(q), "RETURNING")
}

// SkipAppRole keeps connections on the login (owner) role. Only the host-side admin commands
// set it: they change what the API role is not allowed to (workspace trust, passwords).
var SkipAppRole bool

// appRole is the non-owner role the API drops to so that the RLS policies bind it.
const appRole = "quests_app"

type pgConnector struct{ inner driver.Connector }

func (c pgConnector) Connect(ctx context.Context) (driver.Conn, error) {
	cn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	pc := &pgConn{Conn: cn}
	if err := pc.dropToAppRole(ctx); err != nil {
		_ = cn.Close()
		return nil, err
	}
	return pc, nil
}

func (c pgConnector) Driver() driver.Driver { return c.inner.Driver() }

type pgConn struct {
	driver.Conn
	// ws is the app.workspace_id the session currently has ("" = unset, "?" = unknown).
	ws string
}

// dropToAppRole makes RLS apply to this connection. Without the role (a database
// that predates the workspaces migration) the connection stays as it is.
func (c *pgConn) dropToAppRole(ctx context.Context) error {
	if SkipAppRole {
		return nil
	}
	q := c.Conn.(driver.QueryerContext)
	rows, err := q.QueryContext(ctx, "SELECT 1 FROM pg_roles WHERE rolname = '"+appRole+"'", nil)
	if err != nil {
		return err
	}
	dest := make([]driver.Value, 1)
	has := rows.Next(dest) == nil
	_ = rows.Close()
	if !has {
		return nil
	}
	if _, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, "SET ROLE "+appRole, nil); err != nil {
		return fmt.Errorf("set role %s: %w", appRole, err)
	}
	return nil
}

// useWorkspace points the session's app.workspace_id at ctx's workspace.
func (c *pgConn) useWorkspace(ctx context.Context) error {
	want := ""
	if id := wsctx.From(ctx); id > 0 {
		want = strconv.FormatInt(id, 10)
	}
	if c.ws == want {
		return nil
	}
	_, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, "SELECT set_config('app.workspace_id', $1, false)",
		[]driver.NamedValue{{Ordinal: 1, Value: want}})
	if err != nil {
		c.ws = "?"
		return err
	}
	c.ws = want
	return nil
}

func (c *pgConn) Prepare(q string) (driver.Stmt, error) { return c.Conn.Prepare(rebind(q)) }

func (c *pgConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, rebind(q))
}

func (c *pgConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if err := c.useWorkspace(ctx); err != nil {
		return nil, err
	}
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &pgTx{Tx: tx, c: c}, nil
}

// pgTx forgets the tracked workspace on rollback: a set_config issued inside the
// transaction is undone with it.
type pgTx struct {
	driver.Tx
	c *pgConn
}

func (t *pgTx) Rollback() error {
	t.c.ws = "?"
	return t.Tx.Rollback()
}

func (c *pgConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *pgConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *pgConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *pgConn) CheckNamedValue(nv *driver.NamedValue) error {
	// Boolean-ish columns are INTEGER 0/1 in the PG schema (as they are in SQLite), so a
	// Go bool argument is sent as that integer.
	if b, ok := nv.Value.(bool); ok {
		if b {
			nv.Value = int64(1)
		} else {
			nv.Value = int64(0)
		}
		return nil
	}
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (c *pgConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if err := c.useWorkspace(ctx); err != nil {
		return nil, err
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, rebind(q), args)
}

func (c *pgConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if err := c.useWorkspace(ctx); err != nil {
		return nil, err
	}
	if insertReturningID(q) {
		rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, rebind(q)+" RETURNING id", args)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		dest := make([]driver.Value, len(rows.Columns()))
		var res pgResult
		for {
			err := rows.Next(dest)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if res.rows == 0 {
				if id, ok := dest[0].(int64); ok {
					res.id = id
				}
			}
			res.rows++
		}
		return res, nil
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, rebind(q), args)
}

type pgResult struct{ id, rows int64 }

func (r pgResult) LastInsertId() (int64, error) { return r.id, nil }
func (r pgResult) RowsAffected() (int64, error) { return r.rows, nil }

// OpenPostgres opens a pool on a postgres:// URL and checks the schema revision.
func OpenPostgres(url string) (*sql.DB, SchemaStatus, error) {
	var zero SchemaStatus
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, zero, fmt.Errorf("parse database url: %w", err)
	}
	sqlDB := sql.OpenDB(pgConnector{stdlib.GetConnector(*cfg)})
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, zero, err
	}
	st, err := assertMigrated(sqlDB, expectedAlembic)
	if err != nil {
		_ = sqlDB.Close()
		return nil, zero, err
	}
	return sqlDB, st, nil
}
