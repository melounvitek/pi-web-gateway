package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentMigrationCanRollbackCommittedReplacement(t *testing.T) {
	root := t.TempDir()
	store := AttachmentStore{Root: root}
	from, to := "/pending", "/real"
	fromPath := filepath.Join(root, SessionHash(from)+".jsonl")
	toPath := filepath.Join(root, SessionHash(to)+".jsonl")
	if err := os.WriteFile(fromPath, []byte("pending\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toPath, []byte("existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rollback, err := store.Migrate(from, to)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(toPath)
	if err != nil || string(migrated) != "existing\npending\n" {
		t.Fatalf("migrated = %q, %v", migrated, err)
	}
	secondRollback, err := store.Migrate(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if secondRollback != nil {
		t.Fatal("repeated migration was not idempotent")
	}
	unchanged, err := os.ReadFile(toPath)
	if err != nil || string(unchanged) != "existing\npending\n" {
		t.Fatalf("repeated contents = %q, %v", unchanged, err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(toPath)
	if err != nil || string(restored) != "existing\n" {
		t.Fatalf("restored = %q, %v", restored, err)
	}
}
