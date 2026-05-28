package state

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
