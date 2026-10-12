// Package migration upgrades GoChat databases only when explicitly invoked.
package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"strconv"
	"strings"
	"time"

	mysqlscripts "gochat/gochat/mysql"
	"gochat/internal/pkg/config"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const (
	VersionTable  = "gochat_schema_migrations"
	LatestVersion = 5
)

// Open creates a dedicated connection pool. Only mutation commands enable
// multiple SQL statements; the application's pool is never modified.
func Open(ctx context.Context, cfg config.MySQLConfig, writable bool) (*sql.DB, error) {
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 || cfg.Port > 65535 || strings.TrimSpace(cfg.Database) == "" || strings.TrimSpace(cfg.User) == "" {
		return nil, errors.New("mysql host, port (1-65535), user and database are required")
	}
	dsn := drivermysql.NewConfig()
	dsn.User, dsn.Passwd, dsn.DBName = cfg.User, cfg.Password, cfg.Database
	dsn.Net, dsn.Addr = "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dsn.ParseTime, dsn.MultiStatements = true, writable
	dsn.Timeout = 5 * time.Second
	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to mysql: %w", err)
	}
	return db, nil
}

type Runner struct {
	db      *sql.DB
	scripts fs.FS
}

func New(db *sql.DB) *Runner { return &Runner{db: db, scripts: mysqlscripts.Files} }

type Status struct {
	Version     int // -1 means no registered version
	Dirty       bool
	Empty       bool
	Candidate   int // -1 means no complete, contiguous schema was recognized
	Pending     []int
	SchemaError string
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Status does not instantiate the migration driver, which would create its table.
func (r *Runner) Status(ctx context.Context) (Status, error) { return inspectStatus(ctx, r.db) }

func inspectStatus(ctx context.Context, q queryer) (Status, error) {
	s, err := inspectSchema(ctx, q)
	if err != nil {
		return Status{}, err
	}
	status := Status{Version: -1, Candidate: -1, Empty: s.empty()}
	if _, exists := s[VersionTable]; exists {
		rows, err := q.QueryContext(ctx, "SELECT version, dirty FROM `"+VersionTable+"`")
		if err != nil {
			return status, err
		}
		defer rows.Close()
		if rows.Next() {
			if err := rows.Scan(&status.Version, &status.Dirty); err != nil {
				return status, err
			}
			if rows.Next() {
				return status, errors.New("migration table contains multiple version records")
			}
		}
		if err := rows.Err(); err != nil {
			return status, err
		}
	}
	if status.Version < -1 || status.Version > LatestVersion {
		return status, errors.New("database version is not supported by this binary")
	}
	if !status.Empty {
		for version := 0; version <= LatestVersion; version++ {
			if err := s.validate(version); err == nil {
				status.Candidate = version
			}
		}
		if status.Candidate < 0 {
			status.SchemaError = "schema is incomplete or migrations were applied out of order"
		}
		if status.Version >= 0 {
			if err := s.validate(status.Version); err != nil {
				status.SchemaError = err.Error()
			}
		}
	}
	start := status.Version
	if status.Version == -1 {
		start = status.Candidate
		if status.Empty {
			start = 2
		}
	}
	if start >= 0 {
		for v := start + 1; v <= LatestVersion; v++ {
			status.Pending = append(status.Pending, v)
		}
	}
	return status, nil
}

// lockedDriver keeps one advisory lock across preflight, bootstrap and Up.
// The underlying MySQL driver owns and releases the real connection-scoped lock.
type lockedDriver struct {
	database.Driver
	ctx  context.Context
	conn *sql.Conn
}

func (lockedDriver) Lock() error   { return nil }
func (lockedDriver) Unlock() error { return nil }

// The upstream driver's Run uses context.Background. Execute on its already
// locked connection with the command context so cancellation leaves dirty set.
func (d lockedDriver) Run(body io.Reader) error {
	query, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	_, err = d.conn.ExecContext(d.ctx, string(query))
	return err
}

func (r *Runner) withLock(ctx context.Context, action func(*migratemysql.Mysql, *sql.Conn) error) (err error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	driver, err := migratemysql.WithConnection(ctx, conn, &migratemysql.Config{MigrationsTable: VersionTable})
	if err != nil {
		return err
	}
	if err = driver.Lock(); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { err = errors.Join(err, driver.Unlock()) }()
	return action(driver, conn)
}

func (r *Runner) Up(ctx context.Context) error {
	return r.withLock(ctx, func(driver *migratemysql.Mysql, conn *sql.Conn) error {
		status, err := inspectStatus(ctx, conn)
		if err != nil {
			return err
		}
		if status.Dirty {
			return fmt.Errorf("database is dirty at version %d; complete the failed SQL manually before repair", status.Version)
		}
		if status.Version == -1 {
			if !status.Empty {
				return errors.New("existing database has no migration record; inspect status and explicitly register a baseline")
			}
			body, err := fs.ReadFile(r.scripts, "init.sql")
			if err != nil {
				return err
			}
			if err := driver.SetVersion(2, true); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("initialize schema (dirty version 2): %w", err)
			}
			s, err := inspectSchema(ctx, conn)
			if err != nil {
				return err
			}
			if err := s.validate(2); err != nil {
				return err
			}
			if err := driver.SetVersion(2, false); err != nil {
				return err
			}
		} else if status.Empty || status.SchemaError != "" {
			return fmt.Errorf("registered schema does not match version %d: %s", status.Version, status.SchemaError)
		}
		source, err := iofs.New(r.scripts, "migrations")
		if err != nil {
			return err
		}
		defer source.Close()
		m, err := migrate.NewWithInstance("iofs", source, "mysql", lockedDriver{Driver: driver, ctx: ctx, conn: conn})
		if err != nil {
			return err
		}
		err = m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return err
	})
}

// Baseline registers completed historical migrations, without executing SQL.
// Structural checks cannot establish whether historical data repairs ran.
func (r *Runner) Baseline(ctx context.Context, version int) error {
	return r.register(ctx, version, false)
}

// Repair only clears a dirty version after the operator completed its SQL.
func (r *Runner) Repair(ctx context.Context, version int) error {
	return r.register(ctx, version, true)
}

func (r *Runner) register(ctx context.Context, version int, repair bool) error {
	if version < 0 || version > LatestVersion {
		return fmt.Errorf("version must be between 0 and %d", LatestVersion)
	}
	return r.withLock(ctx, func(driver *migratemysql.Mysql, conn *sql.Conn) error {
		status, err := inspectStatus(ctx, conn)
		if err != nil {
			return err
		}
		if repair {
			if !status.Dirty || status.Version != version {
				return errors.New("repair requires the exact current dirty version")
			}
		} else if status.Version != -1 || status.Dirty {
			return errors.New("baseline requires a database without a registered version; use repair for a dirty version")
		}
		s, err := inspectSchema(ctx, conn)
		if err != nil {
			return err
		}
		if err := s.validate(version); err != nil {
			return fmt.Errorf("cannot register version %d: %w", version, err)
		}
		return driver.SetVersion(version, false)
	})
}
