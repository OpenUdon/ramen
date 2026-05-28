package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 1

type Store struct {
	db *sql.DB
}

type ResourceSnapshot struct {
	Address        string
	Type           string
	Provider       string
	DesiredHash    string
	IdentityJSON   string
	AttributesJSON string
	Status         string
	SourceKind     string
	SourceID       string
	OperationID    string
	UpdatedRunID   int64
	UpdatedAt      time.Time
}

func DefaultPath(configDir string) string {
	if strings.TrimSpace(configDir) == "" {
		configDir = "."
	}
	return filepath.Join(configDir, ".ramen", "state.db")
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("state path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("state path is required")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.SchemaReady(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func Init(ctx context.Context, path string) error {
	store, err := Open(ctx, path)
	if err != nil {
		return err
	}
	return store.Close()
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			status TEXT NOT NULL,
			summary_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS resources (
			address TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			provider TEXT,
			desired_hash TEXT,
			identity_json TEXT,
			attributes_json TEXT,
			status TEXT NOT NULL,
			source_kind TEXT,
			source_id TEXT,
			operation_id TEXT,
			updated_run_id INTEGER,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS state_revisions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			resource_address TEXT NOT NULL,
			run_id INTEGER,
			action TEXT NOT NULL,
			before_json TEXT,
			after_json TEXT,
			diff_json TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS locks (
			name TEXT PRIMARY KEY,
			holder TEXT NOT NULL,
			acquired_at TEXT NOT NULL,
			expires_at TEXT
		)`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SchemaReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	row := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, SchemaVersion)
	var one int
	if err := row.Scan(&one); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("state schema version %d is not applied; run ramen init", SchemaVersion)
		}
		return err
	}
	return nil
}

func (s *Store) CurrentResource(ctx context.Context, address string) (*ResourceSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	row := s.db.QueryRowContext(ctx, `SELECT address, type, provider, desired_hash, identity_json, attributes_json, status, source_kind, source_id, operation_id, updated_run_id, updated_at FROM resources WHERE address = ?`, address)
	snap, err := scanSnapshot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return snap, nil
}

func (s *Store) ListCurrentResources(ctx context.Context) ([]ResourceSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT address, type, provider, desired_hash, identity_json, attributes_json, status, source_kind, source_id, operation_id, updated_run_id, updated_at FROM resources ORDER BY address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ResourceSnapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snap)
	}
	return out, rows.Err()
}

func (s *Store) RecordResource(ctx context.Context, snap ResourceSnapshot) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if strings.TrimSpace(snap.Address) == "" {
		return fmt.Errorf("resource address is required")
	}
	if strings.TrimSpace(snap.Status) == "" {
		snap.Status = "managed"
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO resources(address, type, provider, desired_hash, identity_json, attributes_json, status, source_kind, source_id, operation_id, updated_run_id, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(address) DO UPDATE SET
	type = excluded.type,
	provider = excluded.provider,
	desired_hash = excluded.desired_hash,
	identity_json = excluded.identity_json,
	attributes_json = excluded.attributes_json,
	status = excluded.status,
	source_kind = excluded.source_kind,
	source_id = excluded.source_id,
	operation_id = excluded.operation_id,
	updated_run_id = excluded.updated_run_id,
	updated_at = excluded.updated_at`,
		snap.Address, snap.Type, snap.Provider, snap.DesiredHash, snap.IdentityJSON, snap.AttributesJSON, snap.Status, snap.SourceKind, snap.SourceID, snap.OperationID, nullableRunID(snap.UpdatedRunID), snap.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func nullableRunID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

type snapshotScanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(scanner snapshotScanner) (*ResourceSnapshot, error) {
	var snap ResourceSnapshot
	var provider, desiredHash, identityJSON, attributesJSON, sourceKind, sourceID, operationID sql.NullString
	var updatedRunID sql.NullInt64
	var updatedAt string
	if err := scanner.Scan(&snap.Address, &snap.Type, &provider, &desiredHash, &identityJSON, &attributesJSON, &snap.Status, &sourceKind, &sourceID, &operationID, &updatedRunID, &updatedAt); err != nil {
		return nil, err
	}
	snap.Provider = provider.String
	snap.DesiredHash = desiredHash.String
	snap.IdentityJSON = identityJSON.String
	snap.AttributesJSON = attributesJSON.String
	snap.SourceKind = sourceKind.String
	snap.SourceID = sourceID.String
	snap.OperationID = operationID.String
	if updatedRunID.Valid {
		snap.UpdatedRunID = updatedRunID.Int64
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		snap.UpdatedAt = t
	}
	return &snap, nil
}
