package state

import (
	"context"
	"path/filepath"
	"testing"
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

func TestOpenReadOnlyMissingStateReturnsNil(t *testing.T) {
	store, err := OpenReadOnly(context.Background(), filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatalf("OpenReadOnly returned error: %v", err)
	}
	if store != nil {
		t.Fatalf("store = %#v, want nil", store)
	}
}
