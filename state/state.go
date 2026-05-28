package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 2

type Store struct {
	db *sql.DB
}

type Tx struct {
	tx *sql.Tx
}

type ResourceSnapshot struct {
	Address             string
	Type                string
	Provider            string
	DesiredHash         string
	IdentityJSON        string
	AttributesJSON      string
	IdentitySecretRef   string
	AttributesSecretRef string
	Status              string
	SourceKind          string
	SourceID            string
	OperationID         string
	UpdatedRunID        int64
	UpdatedAt           time.Time
}

type Run struct {
	ID          int64
	Command     string
	StartedAt   time.Time
	FinishedAt  time.Time
	Status      string
	SummaryJSON string
}

type Revision struct {
	ID              int64
	ResourceAddress string
	RunID           int64
	Action          string
	BeforeJSON      string
	AfterJSON       string
	DiffJSON        string
	CreatedAt       time.Time
}

type Lock struct {
	Name        string
	Holder      string
	Host        string
	PID         int
	RunID       int64
	AcquiredAt  time.Time
	ExpiresAt   time.Time
	HeartbeatAt time.Time
}

type LockHeldError struct {
	Name        string
	Holder      string
	Host        string
	PID         int
	RunID       int64
	AcquiredAt  time.Time
	ExpiresAt   time.Time
	HeartbeatAt time.Time
}

type LockNotFoundError struct {
	Name string
}

type LockHolderMismatchError struct {
	Name        string
	Holder      string
	Expected    string
	Host        string
	PID         int
	RunID       int64
	AcquiredAt  time.Time
	ExpiresAt   time.Time
	HeartbeatAt time.Time
}

type LockOptions struct {
	Name   string
	Holder string
	TTL    time.Duration
	Host   string
	PID    int
	RunID  int64
}

func (e LockHeldError) Error() string {
	msg := fmt.Sprintf("state lock %q is held by %q since %s", e.Name, e.Holder, e.AcquiredAt.Format(time.RFC3339Nano))
	if e.Host != "" || e.PID != 0 || e.RunID != 0 {
		msg += fmt.Sprintf(" host=%s pid=%d run=%d", e.Host, e.PID, e.RunID)
	}
	if !e.ExpiresAt.IsZero() {
		msg += fmt.Sprintf(" until %s", e.ExpiresAt.Format(time.RFC3339Nano))
	}
	return msg
}

func (e LockNotFoundError) Error() string {
	return fmt.Sprintf("state lock %q is not held", e.Name)
}

func (e LockHolderMismatchError) Error() string {
	msg := fmt.Sprintf("state lock %q is held by %q, not %q", e.Name, e.Holder, e.Expected)
	if e.Host != "" || e.PID != 0 || e.RunID != 0 {
		msg += fmt.Sprintf(" host=%s pid=%d run=%d", e.Host, e.PID, e.RunID)
	}
	if !e.AcquiredAt.IsZero() {
		msg += fmt.Sprintf(" since %s", e.AcquiredAt.Format(time.RFC3339Nano))
	}
	if !e.ExpiresAt.IsZero() {
		msg += fmt.Sprintf(" until %s", e.ExpiresAt.Format(time.RFC3339Nano))
	}
	return msg
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
	configureDB(db)
	store := &Store{db: db}
	if err := configureWriteConnection(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state.integrity_check_failed: %w", err)
	}
	if err := store.CheckIntegrity(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
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
	configureDB(db)
	store := &Store{db: db}
	if err := configureReadConnection(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state.integrity_check_failed: %w", err)
	}
	if err := store.CheckIntegrity(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
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

func configureDB(db *sql.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
}

func configureWriteConnection(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func configureReadConnection(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	applied, maxVersion, err := s.appliedMigrationVersions(ctx)
	if err != nil {
		return err
	}
	if maxVersion > SchemaVersion {
		return fmt.Errorf("state schema version %d is newer than this binary supports (%d)", maxVersion, SchemaVersion)
	}
	for _, migration := range stateMigrations {
		if applied[migration.Version] {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, stmt := range migration.Statements {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply state migration %d: %w", migration.Version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, migration.Version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type migration struct {
	Version    int
	Statements []string
}

var stateMigrations = []migration{
	{
		Version: 1,
		Statements: []string{
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
		},
	},
	{
		Version: 2,
		Statements: []string{
			`ALTER TABLE resources ADD COLUMN identity_secret_ref TEXT`,
			`ALTER TABLE resources ADD COLUMN attributes_secret_ref TEXT`,
			`ALTER TABLE locks ADD COLUMN host TEXT`,
			`ALTER TABLE locks ADD COLUMN pid INTEGER`,
			`ALTER TABLE locks ADD COLUMN run_id INTEGER`,
			`ALTER TABLE locks ADD COLUMN heartbeat_at TEXT`,
		},
	},
}

func (s *Store) appliedMigrationVersions(ctx context.Context) (map[int]bool, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	applied := map[int]bool{}
	maxVersion := 0
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, 0, err
		}
		applied[version] = true
		if version > maxVersion {
			maxVersion = version
		}
	}
	return applied, maxVersion, rows.Err()
}

func (s *Store) CheckIntegrity(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return fmt.Errorf("state.integrity_check_failed: %w", err)
	}
	defer rows.Close()
	var failures []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("state.integrity_check_failed: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			failures = append(failures, result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("state.integrity_check_failed: %w", err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("state.integrity_check_failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if fn == nil {
		return fmt.Errorf("state transaction function is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	wrapped := &Tx{tx: tx}
	if err := fn(wrapped); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) SchemaReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	_, maxVersion, err := s.appliedMigrationVersions(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("state schema version %d is not applied; run ramen init", SchemaVersion)
		}
		return err
	}
	if maxVersion > SchemaVersion {
		return fmt.Errorf("state schema version %d is newer than this binary supports (%d)", maxVersion, SchemaVersion)
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
	row := s.db.QueryRowContext(ctx, resourceSnapshotSelect+` WHERE address = ?`, address)
	snap, err := scanSnapshot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return snap, nil
}

func (tx *Tx) CurrentResource(ctx context.Context, address string) (*ResourceSnapshot, error) {
	if tx == nil || tx.tx == nil {
		return nil, fmt.Errorf("state transaction is nil")
	}
	row := tx.tx.QueryRowContext(ctx, resourceSnapshotSelect+` WHERE address = ?`, address)
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
	rows, err := s.db.QueryContext(ctx, resourceSnapshotSelect+` ORDER BY address`)
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
	return execRecordResource(ctx, s.db, snap)
}

func (tx *Tx) RecordResource(ctx context.Context, snap ResourceSnapshot) error {
	if tx == nil || tx.tx == nil {
		return fmt.Errorf("state transaction is nil")
	}
	return execRecordResource(ctx, tx.tx, snap)
}

func execRecordResource(ctx context.Context, exec sqlExecutor, snap ResourceSnapshot) error {
	if strings.TrimSpace(snap.Address) == "" {
		return fmt.Errorf("resource address is required")
	}
	if strings.TrimSpace(snap.Status) == "" {
		snap.Status = "managed"
	}
	if snap.UpdatedAt.IsZero() {
		snap.UpdatedAt = time.Now().UTC()
	}
	_, err := exec.ExecContext(ctx, `INSERT INTO resources(address, type, provider, desired_hash, identity_json, attributes_json, identity_secret_ref, attributes_secret_ref, status, source_kind, source_id, operation_id, updated_run_id, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(address) DO UPDATE SET
	type = excluded.type,
	provider = excluded.provider,
	desired_hash = excluded.desired_hash,
	identity_json = excluded.identity_json,
	attributes_json = excluded.attributes_json,
	identity_secret_ref = excluded.identity_secret_ref,
	attributes_secret_ref = excluded.attributes_secret_ref,
	status = excluded.status,
	source_kind = excluded.source_kind,
	source_id = excluded.source_id,
	operation_id = excluded.operation_id,
	updated_run_id = excluded.updated_run_id,
	updated_at = excluded.updated_at`,
		snap.Address, snap.Type, snap.Provider, snap.DesiredHash, snap.IdentityJSON, snap.AttributesJSON, nullableString(snap.IdentitySecretRef), nullableString(snap.AttributesSecretRef), snap.Status, snap.SourceKind, snap.SourceID, snap.OperationID, nullableRunID(snap.UpdatedRunID), snap.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) StartRun(ctx context.Context, command string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("state store is nil")
	}
	command = strings.TrimSpace(command)
	if command == "" {
		command = "unknown"
	}
	started := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO runs(command, started_at, status) VALUES(?, ?, ?)`, command, started.Format(time.RFC3339Nano), "running")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishRun(ctx context.Context, id int64, status, summaryJSON string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if id == 0 {
		return nil
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET finished_at = ?, status = ?, summary_json = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), status, nullableString(summaryJSON), id)
	return err
}

func (s *Store) ListRuns(ctx context.Context, status string) ([]Run, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	query := `SELECT id, command, started_at, finished_at, status, summary_json FROM runs`
	var args []any
	if strings.TrimSpace(status) != "" {
		query += ` WHERE status = ?`
		args = append(args, strings.TrimSpace(status))
	}
	query += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (s *Store) MarkAbandonedRuns(ctx context.Context, olderThan time.Duration) ([]Run, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	if olderThan <= 0 {
		return nil, fmt.Errorf("abandoned run age must be positive")
	}
	now := time.Now().UTC()
	before := now.Add(-olderThan)
	rows, err := s.db.QueryContext(ctx, `SELECT id, command, started_at, finished_at, status, summary_json
FROM runs
WHERE status = 'running'
  AND started_at <= ?
  AND NOT EXISTS (
    SELECT 1 FROM locks
    WHERE locks.run_id = runs.id
      AND (locks.expires_at IS NULL OR locks.expires_at > ?)
  )
ORDER BY id`, before.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	var abandoned []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		abandoned = append(abandoned, *run)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(abandoned) == 0 {
		return nil, nil
	}
	finishedAt := now.Format(time.RFC3339Nano)
	for _, run := range abandoned {
		if _, err := s.db.ExecContext(ctx, `UPDATE runs SET finished_at = ?, status = ?, summary_json = COALESCE(summary_json, ?) WHERE id = ? AND status = 'running'`, finishedAt, "abandoned", `{"abandoned":true}`, run.ID); err != nil {
			return nil, err
		}
	}
	return abandoned, nil
}

func (s *Store) RecordRevision(ctx context.Context, rev Revision) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if strings.TrimSpace(rev.ResourceAddress) == "" {
		return fmt.Errorf("resource address is required")
	}
	if strings.TrimSpace(rev.Action) == "" {
		return fmt.Errorf("revision action is required")
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = time.Now().UTC()
	}
	return execRecordRevision(ctx, s.db, rev)
}

func (tx *Tx) RecordRevision(ctx context.Context, rev Revision) error {
	if tx == nil || tx.tx == nil {
		return fmt.Errorf("state transaction is nil")
	}
	return execRecordRevision(ctx, tx.tx, rev)
}

func execRecordRevision(ctx context.Context, exec sqlExecutor, rev Revision) error {
	if strings.TrimSpace(rev.ResourceAddress) == "" {
		return fmt.Errorf("resource address is required")
	}
	if strings.TrimSpace(rev.Action) == "" {
		return fmt.Errorf("revision action is required")
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = time.Now().UTC()
	}
	_, err := exec.ExecContext(ctx, `INSERT INTO state_revisions(resource_address, run_id, action, before_json, after_json, diff_json, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		rev.ResourceAddress, nullableRunID(rev.RunID), rev.Action, nullableString(rev.BeforeJSON), nullableString(rev.AfterJSON), nullableString(rev.DiffJSON), rev.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListRevisions(ctx context.Context, address string) ([]Revision, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	query := `SELECT id, resource_address, run_id, action, before_json, after_json, diff_json, created_at FROM state_revisions`
	var args []any
	if strings.TrimSpace(address) != "" {
		query += ` WHERE resource_address = ?`
		args = append(args, address)
	}
	query += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Revision
	for rows.Next() {
		var rev Revision
		var runID sql.NullInt64
		var beforeJSON, afterJSON, diffJSON sql.NullString
		var createdAt string
		if err := rows.Scan(&rev.ID, &rev.ResourceAddress, &runID, &rev.Action, &beforeJSON, &afterJSON, &diffJSON, &createdAt); err != nil {
			return nil, err
		}
		if runID.Valid {
			rev.RunID = runID.Int64
		}
		rev.BeforeJSON = beforeJSON.String
		rev.AfterJSON = afterJSON.String
		rev.DiffJSON = diffJSON.String
		if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			rev.CreatedAt = t
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (s *Store) DeleteResource(ctx context.Context, address string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("resource address is required")
	}
	return execDeleteResource(ctx, s.db, address)
}

func (tx *Tx) DeleteResource(ctx context.Context, address string) error {
	if tx == nil || tx.tx == nil {
		return fmt.Errorf("state transaction is nil")
	}
	return execDeleteResource(ctx, tx.tx, address)
}

func execDeleteResource(ctx context.Context, exec sqlExecutor, address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("resource address is required")
	}
	_, err := exec.ExecContext(ctx, `DELETE FROM resources WHERE address = ?`, address)
	return err
}

func (s *Store) AcquireLock(ctx context.Context, name, holder string, ttl time.Duration) error {
	return s.AcquireLockWithOptions(ctx, LockOptions{Name: name, Holder: holder, TTL: ttl})
}

func (s *Store) AcquireLockWithOptions(ctx context.Context, opts LockOptions) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	name := strings.TrimSpace(opts.Name)
	holder := strings.TrimSpace(opts.Holder)
	if name == "" {
		return fmt.Errorf("lock name is required")
	}
	if holder == "" {
		return fmt.Errorf("lock holder is required")
	}
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host, _ = os.Hostname()
	}
	pid := opts.PID
	if pid == 0 {
		pid = os.Getpid()
	}
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM locks WHERE name = ? AND expires_at IS NOT NULL AND expires_at <= ?`, name, now.Format(time.RFC3339Nano))
	var expires any
	if opts.TTL > 0 {
		expires = now.Add(opts.TTL).Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO locks(name, holder, acquired_at, expires_at, host, pid, run_id, heartbeat_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, name, holder, now.Format(time.RFC3339Nano), expires, nullableString(host), nullableInt(pid), nullableRunID(opts.RunID), now.Format(time.RFC3339Nano))
	if err != nil {
		if held, heldErr := s.currentLock(ctx, name); heldErr == nil && held != nil {
			return LockHeldError{Name: held.Name, Holder: held.Holder, Host: held.Host, PID: held.PID, RunID: held.RunID, AcquiredAt: held.AcquiredAt, ExpiresAt: held.ExpiresAt, HeartbeatAt: held.HeartbeatAt}
		}
		return err
	}
	return nil
}

func (s *Store) AttachLockRun(ctx context.Context, name, holder string, runID int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	name = strings.TrimSpace(name)
	holder = strings.TrimSpace(holder)
	if name == "" {
		return fmt.Errorf("lock name is required")
	}
	if holder == "" {
		return fmt.Errorf("lock holder is required")
	}
	if runID == 0 {
		return fmt.Errorf("run id is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE locks SET run_id = ? WHERE name = ? AND holder = ?`, runID, name, holder)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		lock, lockErr := s.CurrentLock(ctx, name)
		if lockErr != nil {
			return lockErr
		}
		if lock == nil {
			return LockNotFoundError{Name: name}
		}
		return LockHolderMismatchError{Name: lock.Name, Holder: lock.Holder, Expected: holder, Host: lock.Host, PID: lock.PID, RunID: lock.RunID, AcquiredAt: lock.AcquiredAt, ExpiresAt: lock.ExpiresAt, HeartbeatAt: lock.HeartbeatAt}
	}
	return nil
}

func (s *Store) RenewLock(ctx context.Context, name, holder string, ttl time.Duration) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	name = strings.TrimSpace(name)
	holder = strings.TrimSpace(holder)
	if name == "" {
		return fmt.Errorf("lock name is required")
	}
	if holder == "" {
		return fmt.Errorf("lock holder is required")
	}
	now := time.Now().UTC()
	var expires any
	if ttl > 0 {
		expires = now.Add(ttl).Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE locks SET expires_at = ?, heartbeat_at = ? WHERE name = ? AND holder = ? AND (expires_at IS NULL OR expires_at > ?)`, expires, now.Format(time.RFC3339Nano), name, holder, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		lock, lockErr := s.CurrentLock(ctx, name)
		if lockErr != nil {
			return lockErr
		}
		if lock == nil {
			return LockNotFoundError{Name: name}
		}
		return LockHolderMismatchError{Name: lock.Name, Holder: lock.Holder, Expected: holder, Host: lock.Host, PID: lock.PID, RunID: lock.RunID, AcquiredAt: lock.AcquiredAt, ExpiresAt: lock.ExpiresAt, HeartbeatAt: lock.HeartbeatAt}
	}
	return nil
}

func (s *Store) StartLockRenewal(ctx context.Context, name, holder string, ttl, interval time.Duration) func() {
	if ttl <= 0 {
		return func() {}
	}
	if interval <= 0 || interval >= ttl {
		interval = ttl / 3
	}
	if interval <= 0 {
		interval = time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}
	renewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				_ = s.RenewLock(context.Background(), name, holder, ttl)
			}
		}
	}()
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (s *Store) CurrentLock(ctx context.Context, name string) (*Lock, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("lock name is required")
	}
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM locks WHERE name = ? AND expires_at IS NOT NULL AND expires_at <= ?`, name, now.Format(time.RFC3339Nano))
	return s.currentLock(ctx, name)
}

func (s *Store) currentLock(ctx context.Context, name string) (*Lock, error) {
	row := s.db.QueryRowContext(ctx, `SELECT name, holder, acquired_at, expires_at, host, pid, run_id, heartbeat_at FROM locks WHERE name = ?`, name)
	var lock Lock
	var acquiredAt string
	var expiresAt, host, heartbeatAt sql.NullString
	var pid, runID sql.NullInt64
	if err := row.Scan(&lock.Name, &lock.Holder, &acquiredAt, &expiresAt, &host, &pid, &runID, &heartbeatAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	lock.Host = host.String
	if pid.Valid {
		lock.PID = int(pid.Int64)
	}
	if runID.Valid {
		lock.RunID = runID.Int64
	}
	if t, err := time.Parse(time.RFC3339Nano, acquiredAt); err == nil {
		lock.AcquiredAt = t
	}
	if expiresAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, expiresAt.String); err == nil {
			lock.ExpiresAt = t
		}
	}
	if heartbeatAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, heartbeatAt.String); err == nil {
			lock.HeartbeatAt = t
		}
	}
	return &lock, nil
}

func (s *Store) ForceUnlock(ctx context.Context, name, holder string) (*Lock, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("state store is nil")
	}
	name = strings.TrimSpace(name)
	holder = strings.TrimSpace(holder)
	if name == "" {
		return nil, fmt.Errorf("lock name is required")
	}
	if holder == "" {
		return nil, fmt.Errorf("lock holder is required")
	}
	lock, err := s.CurrentLock(ctx, name)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, LockNotFoundError{Name: name}
	}
	if lock.Holder != holder {
		return nil, LockHolderMismatchError{Name: lock.Name, Holder: lock.Holder, Expected: holder, Host: lock.Host, PID: lock.PID, RunID: lock.RunID, AcquiredAt: lock.AcquiredAt, ExpiresAt: lock.ExpiresAt, HeartbeatAt: lock.HeartbeatAt}
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM locks WHERE name = ? AND holder = ?`, name, holder)
	if err != nil {
		return nil, err
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return nil, LockNotFoundError{Name: name}
	}
	return lock, nil
}

func (s *Store) ReleaseLock(ctx context.Context, name, holder string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state store is nil")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM locks WHERE name = ? AND holder = ?`, strings.TrimSpace(name), strings.TrimSpace(holder))
	return err
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableRunID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

type snapshotScanner interface {
	Scan(dest ...any) error
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

const resourceSnapshotSelect = `SELECT address, type, provider, desired_hash, identity_json, attributes_json, identity_secret_ref, attributes_secret_ref, status, source_kind, source_id, operation_id, updated_run_id, updated_at FROM resources`

func scanSnapshot(scanner snapshotScanner) (*ResourceSnapshot, error) {
	var snap ResourceSnapshot
	var provider, desiredHash, identityJSON, attributesJSON, identitySecretRef, attributesSecretRef, sourceKind, sourceID, operationID sql.NullString
	var updatedRunID sql.NullInt64
	var updatedAt string
	if err := scanner.Scan(&snap.Address, &snap.Type, &provider, &desiredHash, &identityJSON, &attributesJSON, &identitySecretRef, &attributesSecretRef, &snap.Status, &sourceKind, &sourceID, &operationID, &updatedRunID, &updatedAt); err != nil {
		return nil, err
	}
	snap.Provider = provider.String
	snap.DesiredHash = desiredHash.String
	snap.IdentityJSON = identityJSON.String
	snap.AttributesJSON = attributesJSON.String
	snap.IdentitySecretRef = identitySecretRef.String
	snap.AttributesSecretRef = attributesSecretRef.String
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

func scanRun(scanner snapshotScanner) (*Run, error) {
	var run Run
	var finishedAt, summaryJSON sql.NullString
	var startedAt string
	if err := scanner.Scan(&run.ID, &run.Command, &startedAt, &finishedAt, &run.Status, &summaryJSON); err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339Nano, startedAt); err == nil {
		run.StartedAt = t
	}
	if finishedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, finishedAt.String); err == nil {
			run.FinishedAt = t
		}
	}
	run.SummaryJSON = summaryJSON.String
	return &run, nil
}
