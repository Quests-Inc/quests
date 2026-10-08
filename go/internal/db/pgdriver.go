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
)

// The store layer is written in SQLite dialect (`?` placeholders,
// Result.LastInsertId). Rather than touch ~150 call sites, the PostgreSQL
// connection is wrapped so that
//   - `?` placeholders become `$1, $2, …` (quotes respected), and
//   - a plain `INSERT INTO t …` gets `RETURNING id` appended and its
//     LastInsertId is served from the returned row.
//
// Everything else must be portable SQL (ON CONFLICT, LIKE, …).

// noIDTables are insert targets without a serial `id` column.
var noIDTables = map[string]bool{
	"quest_tag":       true,
	"template_tag":    true,
	"alembic_version": true,
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

type pgConnector struct{ inner driver.Connector }

func (c pgConnector) Connect(ctx context.Context) (driver.Conn, error) {
	cn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &pgConn{cn}, nil
}

func (c pgConnector) Driver() driver.Driver { return c.inner.Driver() }

type pgConn struct{ driver.Conn }

func (c *pgConn) Prepare(q string) (driver.Stmt, error) { return c.Conn.Prepare(rebind(q)) }

func (c *pgConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, rebind(q))
}

func (c *pgConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
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
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (c *pgConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, rebind(q), args)
}

func (c *pgConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
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
	st, err := assertMigrated(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, zero, err
	}
	return sqlDB, st, nil
}
