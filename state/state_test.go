package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestInitCreatesSchemaAndRecordResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ramen", "state.db")
	if err := Init(context.Background(), path); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	store, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	store, err = Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := store.RecordResource(context.Background(), ResourceSnapshot{
		Address:     "aws_iam_role.role",
		Type:        "aws_iam_role",
		DesiredHash: "sha256:test",
		Status:      "managed",
	}); err != nil {
		t.Fatalf("RecordResource returned error: %v", err)
	}
	got, err := store.CurrentResource(context.Background(), "aws_iam_role.role")
	if err != nil {
		t.Fatalf("CurrentResource returned error: %v", err)
	}
	if got == nil || got.DesiredHash != "sha256:test" || got.Status != "managed" {
		t.Fatalf("snapshot = %#v", got)
	}
	all, err := store.ListCurrentResources(context.Background())
	if err != nil {
		t.Fatalf("ListCurrentResources returned error: %v", err)
	}
	if len(all) != 1 || all[0].Address != "aws_iam_role.role" {
		t.Fatalf("resources = %#v", all)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestStoreRevisionsAndLocks(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.RecordRevision(ctx, Revision{ResourceAddress: "aws_iam_role.role", Action: "create", AfterJSON: `{"ok":true}`}); err != nil {
		t.Fatalf("record revision: %v", err)
	}
	revisions, err := store.ListRevisions(ctx, "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Action != "create" || revisions[0].AfterJSON == "" {
		t.Fatalf("revisions = %#v", revisions)
	}
	if err := store.AcquireLock(ctx, "state", "holder-1", time.Minute); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	if err := second.AcquireLock(ctx, "state", "holder-2", time.Minute); err == nil {
		t.Fatalf("second lock unexpectedly acquired")
	} else {
		var held LockHeldError
		if !errors.As(err, &held) {
			t.Fatalf("lock error = %T %[1]v, want LockHeldError", err)
		}
		if held.Holder != "holder-1" || held.AcquiredAt.IsZero() || !strings.Contains(err.Error(), "holder-1") {
			t.Fatalf("lock detail = %#v error=%v", held, err)
		}
	}
	if err := store.ReleaseLock(ctx, "state", "holder-1"); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if err := second.AcquireLock(ctx, "state", "holder-2", time.Minute); err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	_ = second.Close()
	_ = store.Close()
}

func TestForceUnlockRequiresExactHolderAndPrunesExpiredLocks(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.AcquireLock(ctx, "state", "holder-1", time.Minute); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if _, err := store.ForceUnlock(ctx, "state", "wrong-holder"); err == nil {
		t.Fatalf("force unlock succeeded with wrong holder")
	} else {
		var mismatch LockHolderMismatchError
		if !errors.As(err, &mismatch) || mismatch.Holder != "holder-1" || mismatch.Expected != "wrong-holder" {
			t.Fatalf("mismatch error = %#v %[1]v", err)
		}
	}
	lock, err := store.ForceUnlock(ctx, "state", "holder-1")
	if err != nil {
		t.Fatalf("force unlock: %v", err)
	}
	if lock.Holder != "holder-1" || lock.AcquiredAt.IsZero() {
		t.Fatalf("unlocked lock = %#v", lock)
	}
	if current, err := store.CurrentLock(ctx, "state"); err != nil || current != nil {
		t.Fatalf("current lock after unlock = %#v err=%v", current, err)
	}
	if _, err := store.ForceUnlock(ctx, "state", "holder-1"); err == nil {
		t.Fatalf("force unlock unexpectedly found missing lock")
	} else {
		var missing LockNotFoundError
		if !errors.As(err, &missing) {
			t.Fatalf("missing error = %T %[1]v", err)
		}
	}
	if err := store.AcquireLock(ctx, "state", "expired-holder", time.Nanosecond); err != nil {
		t.Fatalf("acquire expiring lock: %v", err)
	}
	time.Sleep(time.Millisecond)
	if current, err := store.CurrentLock(ctx, "state"); err != nil || current != nil {
		t.Fatalf("expired lock should be pruned, got %#v err=%v", current, err)
	}
	_ = store.Close()
}

func TestWithTxRollsBackResourceAndRevision(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = store.WithTx(ctx, func(tx *Tx) error {
		if err := tx.RecordResource(ctx, ResourceSnapshot{
			Address:     "aws_iam_role.role",
			Type:        "aws_iam_role",
			DesiredHash: "sha256:test",
			Status:      "managed",
		}); err != nil {
			return err
		}
		if err := tx.RecordRevision(ctx, Revision{ResourceAddress: "aws_iam_role.role", Action: "create"}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("WithTx succeeded unexpectedly")
	}
	snap, err := store.CurrentResource(ctx, "aws_iam_role.role")
	if err != nil {
		t.Fatalf("current resource: %v", err)
	}
	if snap != nil {
		t.Fatalf("resource committed despite rollback: %#v", snap)
	}
	revs, err := store.ListRevisions(ctx, "aws_iam_role.role")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revs) != 0 {
		t.Fatalf("revisions committed despite rollback: %#v", revs)
	}
	_ = store.Close()
}

func TestOpenReadOnlyMissingStateReturnsNil(t *testing.T) {
	store, err := OpenReadOnly(context.Background(), filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatalf("OpenReadOnly returned error: %v", err)
	}
	if store != nil {
		t.Fatalf("store = %#v, want nil", store)
	}
}

func TestWorkspacePathAndAuditDigest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspacePath, err := WorkspacePath(root, "prod")
	if err != nil {
		t.Fatalf("workspace path: %v", err)
	}
	if !strings.Contains(workspacePath, filepath.Join(".ramen", "workspaces", "prod", "state.db")) {
		t.Fatalf("workspace path = %s", workspacePath)
	}
	if _, err := WorkspacePath(root, "../prod"); err == nil {
		t.Fatalf("invalid workspace unexpectedly accepted")
	}
	store, err := Open(ctx, workspacePath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.RecordResource(ctx, ResourceSnapshot{Address: "example.one", Type: "example", DesiredHash: "sha256:one", Status: "managed"}); err != nil {
		t.Fatalf("record resource: %v", err)
	}
	runID, err := store.StartRun(ctx, "apply")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.RecordRevision(ctx, Revision{ResourceAddress: "example.one", RunID: runID, Action: "create", AfterJSON: `{"ok":true}`}); err != nil {
		t.Fatalf("record revision: %v", err)
	}
	audit, err := store.Audit(ctx)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if audit.Version != AuditVersion || !strings.HasPrefix(audit.Digest, "sha256:") || audit.Counts["resources"] != 1 || audit.Counts["revisions"] != 1 || audit.Counts["runs"] != 1 {
		t.Fatalf("audit = %#v", audit)
	}
	_ = store.Close()
}

func TestMigrateUpgradesV1StateAndPreservesResources(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(1, '2026-01-01T00:00:00Z')`,
		`CREATE TABLE runs (id INTEGER PRIMARY KEY AUTOINCREMENT, command TEXT NOT NULL, started_at TEXT NOT NULL, finished_at TEXT, status TEXT NOT NULL, summary_json TEXT)`,
		`CREATE TABLE resources (address TEXT PRIMARY KEY, type TEXT NOT NULL, provider TEXT, desired_hash TEXT, identity_json TEXT, attributes_json TEXT, status TEXT NOT NULL, source_kind TEXT, source_id TEXT, operation_id TEXT, updated_run_id INTEGER, updated_at TEXT NOT NULL)`,
		`CREATE TABLE state_revisions (id INTEGER PRIMARY KEY AUTOINCREMENT, resource_address TEXT NOT NULL, run_id INTEGER, action TEXT NOT NULL, before_json TEXT, after_json TEXT, diff_json TEXT, created_at TEXT NOT NULL)`,
		`CREATE TABLE locks (name TEXT PRIMARY KEY, holder TEXT NOT NULL, acquired_at TEXT NOT NULL, expires_at TEXT)`,
		`INSERT INTO resources(address, type, desired_hash, status, updated_at) VALUES('example.one', 'example', 'sha256:v1', 'managed', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	snap, err := store.CurrentResource(ctx, "example.one")
	if err != nil {
		t.Fatalf("CurrentResource returned error: %v", err)
	}
	if snap == nil || snap.DesiredHash != "sha256:v1" {
		t.Fatalf("snapshot after migration = %#v", snap)
	}
	if err := store.RecordResource(ctx, ResourceSnapshot{Address: "example.one", Type: "example", DesiredHash: "sha256:v2", IdentitySecretRef: "secret://identity", AttributesSecretRef: "secret://attrs", Status: "managed"}); err != nil {
		t.Fatalf("RecordResource with secret refs returned error: %v", err)
	}
	snap, err = store.CurrentResource(ctx, "example.one")
	if err != nil {
		t.Fatalf("CurrentResource after secret refs returned error: %v", err)
	}
	if snap.IdentitySecretRef != "secret://identity" || snap.AttributesSecretRef != "secret://attrs" {
		t.Fatalf("secret refs not preserved: %#v", snap)
	}
	if err := store.SchemaReady(ctx); err != nil {
		t.Fatalf("SchemaReady returned error: %v", err)
	}
	_ = store.Close()
}

func TestOpenRejectsNewerStateSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, '2026-01-01T00:00:00Z')`, SchemaVersion+1); err != nil {
		t.Fatalf("insert newer migration: %v", err)
	}
	_ = db.Close()

	_, err = Open(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "newer than this binary") {
		t.Fatalf("Open error = %v, want newer schema rejection", err)
	}
	_, err = OpenReadOnly(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "newer than this binary") {
		t.Fatalf("OpenReadOnly error = %v, want newer schema rejection", err)
	}
}

func TestOpenRejectsCorruptStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}
	_, err := Open(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "state.integrity_check_failed") {
		t.Fatalf("Open corrupt error = %v", err)
	}
}

func TestLockMetadataRenewalAndRunAttachment(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.AcquireLockWithOptions(ctx, LockOptions{Name: "state", Holder: "holder", TTL: 80 * time.Millisecond, Host: "host-a", PID: 123}); err != nil {
		t.Fatalf("AcquireLockWithOptions: %v", err)
	}
	runID, err := store.StartRun(ctx, "apply")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := store.AttachLockRun(ctx, "state", "holder", runID); err != nil {
		t.Fatalf("AttachLockRun: %v", err)
	}
	lock, err := store.CurrentLock(ctx, "state")
	if err != nil {
		t.Fatalf("CurrentLock: %v", err)
	}
	if lock == nil || lock.Host != "host-a" || lock.PID != 123 || lock.RunID != runID || lock.HeartbeatAt.IsZero() {
		t.Fatalf("lock metadata = %#v", lock)
	}
	initialExpiry := lock.ExpiresAt
	stop := store.StartLockRenewal(ctx, "state", "holder", 80*time.Millisecond, 10*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	stop()
	lock, err = store.CurrentLock(ctx, "state")
	if err != nil {
		t.Fatalf("CurrentLock after renewal: %v", err)
	}
	if lock == nil || !lock.ExpiresAt.After(initialExpiry) {
		t.Fatalf("lock was not renewed: initial=%s current=%#v", initialExpiry, lock)
	}
	_ = store.Close()
}

func TestAttachLockRunReportsHolderMismatch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.AcquireLockWithOptions(ctx, LockOptions{Name: "state", Holder: "holder-a", TTL: time.Hour, Host: "host-a", PID: 123}); err != nil {
		t.Fatalf("AcquireLockWithOptions: %v", err)
	}
	runID, err := store.StartRun(ctx, "apply")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	err = store.AttachLockRun(ctx, "state", "holder-b", runID)
	var mismatch LockHolderMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("AttachLockRun error = %T %[1]v, want LockHolderMismatchError", err)
	}
	if mismatch.Holder != "holder-a" || mismatch.Expected != "holder-b" || mismatch.Host != "host-a" || mismatch.PID != 123 {
		t.Fatalf("mismatch metadata = %#v", mismatch)
	}
	_ = store.Close()
}

func TestRenewLockPrunesExpiredLock(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.AcquireLock(ctx, "state", "holder", time.Nanosecond); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	time.Sleep(time.Millisecond)
	err = store.RenewLock(ctx, "state", "holder", time.Minute)
	var missing LockNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("RenewLock error = %T %[1]v, want LockNotFoundError", err)
	}
	if lock, err := store.CurrentLock(ctx, "state"); err != nil || lock != nil {
		t.Fatalf("CurrentLock after expired renewal = %#v err=%v", lock, err)
	}
	_ = store.Close()
}

func TestBackupRestoreExportAndVacuum(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	backupPath := filepath.Join(root, "backup.db")
	restorePath := filepath.Join(root, "restored.db")
	store, err := Open(ctx, statePath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.RecordResource(ctx, ResourceSnapshot{Address: "example.one", Type: "example", DesiredHash: "sha256:test", Status: "managed"}); err != nil {
		t.Fatalf("RecordResource: %v", err)
	}
	if err := store.RecordRevision(ctx, Revision{ResourceAddress: "example.one", Action: "create"}); err != nil {
		t.Fatalf("RecordRevision: %v", err)
	}
	runID, err := store.StartRun(ctx, "apply")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if err := store.FinishRun(ctx, runID, "completed", `{"ok":true}`); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if err := store.AcquireLockWithOptions(ctx, LockOptions{Name: "maintenance", Holder: "test", TTL: time.Hour, RunID: runID}); err != nil {
		t.Fatalf("AcquireLockWithOptions: %v", err)
	}
	doc, err := store.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if doc.Version != ExportVersion || doc.SchemaVersion != SchemaVersion || len(doc.Migrations) == 0 || len(doc.Resources) != 1 || len(doc.Revisions) != 1 || len(doc.Runs) != 1 || len(doc.Locks) != 1 {
		t.Fatalf("export document = %#v", doc)
	}
	if err := store.Backup(ctx, backupPath); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if err := store.Vacuum(ctx); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
	_ = store.Close()

	if err := Restore(ctx, restorePath, backupPath, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Restore without force error = %v, want force requirement", err)
	}
	if err := Restore(ctx, restorePath, backupPath, true); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restored, err := OpenReadOnly(ctx, restorePath)
	if err != nil {
		t.Fatalf("OpenReadOnly restored: %v", err)
	}
	snap, err := restored.CurrentResource(ctx, "example.one")
	if err != nil {
		t.Fatalf("CurrentResource restored: %v", err)
	}
	if snap == nil || snap.DesiredHash != "sha256:test" {
		t.Fatalf("restored snapshot = %#v", snap)
	}
	_ = restored.Close()
}

func TestMarkAbandonedRunsSkipsActiveLockedRuns(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	abandonedID, err := store.StartRun(ctx, "apply")
	if err != nil {
		t.Fatalf("StartRun abandoned: %v", err)
	}
	activeID, err := store.StartRun(ctx, "destroy")
	if err != nil {
		t.Fatalf("StartRun active: %v", err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `UPDATE runs SET started_at = ? WHERE id IN (?, ?)`, old, abandonedID, activeID); err != nil {
		t.Fatalf("age runs: %v", err)
	}
	if err := store.AcquireLockWithOptions(ctx, LockOptions{Name: "state", Holder: "active", TTL: time.Hour, RunID: activeID}); err != nil {
		t.Fatalf("AcquireLockWithOptions active: %v", err)
	}
	abandoned, err := store.MarkAbandonedRuns(ctx, time.Hour)
	if err != nil {
		t.Fatalf("MarkAbandonedRuns: %v", err)
	}
	if len(abandoned) != 1 || abandoned[0].ID != abandonedID {
		t.Fatalf("abandoned runs = %#v", abandoned)
	}
	running, err := store.ListRuns(ctx, "running")
	if err != nil {
		t.Fatalf("ListRuns running: %v", err)
	}
	if len(running) != 1 || running[0].ID != activeID {
		t.Fatalf("running runs = %#v", running)
	}
	abandonedRuns, err := store.ListRuns(ctx, "abandoned")
	if err != nil {
		t.Fatalf("ListRuns abandoned: %v", err)
	}
	if len(abandonedRuns) != 1 || abandonedRuns[0].ID != abandonedID || abandonedRuns[0].FinishedAt.IsZero() {
		t.Fatalf("abandoned status rows = %#v", abandonedRuns)
	}
	_ = store.Close()
}
