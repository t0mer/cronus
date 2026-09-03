// Package store is Cronus's persistence layer over SQLite (pure-Go
// modernc.org/sqlite, CGO-free). It stores monitored servers and their
// measurement history, applies embedded migrations at startup, and prunes
// measurements past the retention window. All access uses prepared statements
// via database/sql.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Store is a handle to the SQLite database.
type Store struct {
	db *sql.DB
}

// Server is a saved (monitored) NTP server.
type Server struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Label     string    `json:"label"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Measurement is a single stored poll result for a server.
type Measurement struct {
	ID          int64         `json:"id"`
	ServerID    string        `json:"server_id"`
	TS          time.Time     `json:"ts"`
	Reachable   bool          `json:"reachable"`
	Offset      time.Duration `json:"offset"`
	RTT         time.Duration `json:"rtt"`
	Jitter      time.Duration `json:"jitter"`
	Stratum     uint8         `json:"stratum"`
	Leap        uint8         `json:"leap"`
	ResolvedIP  string        `json:"resolved_ip"`
	ReferenceID string        `json:"reference_id"`
	Error       string        `json:"error,omitempty"`
}

// DBStats summarises database contents.
type DBStats struct {
	Servers      int `json:"servers"`
	Measurements int `json:"measurements"`
}

// Open opens (creating if needed) the SQLite database at path and applies all
// embedded migrations. Use ":memory:" for an in-memory database.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single writer avoids "database is locked" under concurrent writes with
	// the pure-Go driver; reads still stream fine for this workload.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate applies any embedded migrations not yet recorded, in filename order.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// CreateServer inserts a new server, assigning an ID and timestamps.
func (s *Store) CreateServer(ctx context.Context, srv Server) (Server, error) {
	srv.ID = uuid.NewString()
	now := time.Now().UTC()
	srv.CreatedAt, srv.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO servers (id, address, label, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		srv.ID, srv.Address, srv.Label, boolToInt(srv.Enabled), srv.CreatedAt, srv.UpdatedAt)
	if err != nil {
		return Server{}, fmt.Errorf("insert server: %w", err)
	}
	return srv, nil
}

// ListServers returns all servers ordered by creation time.
func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, address, label, enabled, created_at, updated_at
		 FROM servers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		srv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

// EnabledServers returns only the servers with enabled = true.
func (s *Store) EnabledServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, address, label, enabled, created_at, updated_at
		 FROM servers WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		srv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

// GetServer returns a single server by ID, or ErrNotFound.
func (s *Store) GetServer(ctx context.Context, id string) (Server, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, address, label, enabled, created_at, updated_at
		 FROM servers WHERE id = ?`, id)
	srv, err := scanServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return srv, err
}

// UpdateServer updates the mutable fields (address, label, enabled) of a server.
func (s *Store) UpdateServer(ctx context.Context, srv Server) (Server, error) {
	srv.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE servers SET address = ?, label = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		srv.Address, srv.Label, boolToInt(srv.Enabled), srv.UpdatedAt, srv.ID)
	if err != nil {
		return Server{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Server{}, ErrNotFound
	}
	return s.GetServer(ctx, srv.ID)
}

// DeleteServer removes a server and (via ON DELETE CASCADE) its measurements.
func (s *Store) DeleteServer(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertMeasurement stores one measurement.
func (s *Store) InsertMeasurement(ctx context.Context, m Measurement) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO measurements
		 (server_id, ts, reachable, offset_seconds, rtt_seconds, jitter_seconds,
		  stratum, leap, resolved_ip, reference_id, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ServerID, m.TS.UTC(), boolToInt(m.Reachable),
		m.Offset.Seconds(), m.RTT.Seconds(), m.Jitter.Seconds(),
		m.Stratum, m.Leap, m.ResolvedIP, m.ReferenceID, m.Error)
	if err != nil {
		return fmt.Errorf("insert measurement: %w", err)
	}
	return nil
}

// Measurements returns measurements for a server within [from, to] (either may
// be zero for open-ended), oldest first, capped at limit (0 = no cap).
func (s *Store) Measurements(ctx context.Context, serverID string, from, to time.Time, limit int) ([]Measurement, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT id, server_id, ts, reachable, offset_seconds, rtt_seconds,
		jitter_seconds, stratum, leap, resolved_ip, reference_id, error
		FROM measurements WHERE server_id = ?`)
	args := []any{serverID}
	if !from.IsZero() {
		q.WriteString(" AND ts >= ?")
		args = append(args, from.UTC())
	}
	if !to.IsZero() {
		q.WriteString(" AND ts <= ?")
		args = append(args, to.UTC())
	}
	q.WriteString(" ORDER BY ts")
	if limit > 0 {
		q.WriteString(" LIMIT ?")
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Measurement
	for rows.Next() {
		m, err := scanMeasurement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneMeasurements deletes measurements older than before, returning the count.
func (s *Store) PruneMeasurements(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM measurements WHERE ts < ?`, before.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats returns row counts for the status endpoint.
func (s *Store) Stats(ctx context.Context) (DBStats, error) {
	var st DBStats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM servers`).Scan(&st.Servers); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM measurements`).Scan(&st.Measurements); err != nil {
		return st, err
	}
	return st, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanServer(sc scanner) (Server, error) {
	var srv Server
	var enabled int
	if err := sc.Scan(&srv.ID, &srv.Address, &srv.Label, &enabled, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
		return Server{}, err
	}
	srv.Enabled = enabled != 0
	return srv, nil
}

func scanMeasurement(sc scanner) (Measurement, error) {
	var m Measurement
	var reachable int
	var offsetS, rttS, jitterS float64
	if err := sc.Scan(&m.ID, &m.ServerID, &m.TS, &reachable, &offsetS, &rttS, &jitterS,
		&m.Stratum, &m.Leap, &m.ResolvedIP, &m.ReferenceID, &m.Error); err != nil {
		return Measurement{}, err
	}
	m.Reachable = reachable != 0
	m.Offset = time.Duration(offsetS * float64(time.Second))
	m.RTT = time.Duration(rttS * float64(time.Second))
	m.Jitter = time.Duration(jitterS * float64(time.Second))
	return m, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
