package condition

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileConditionStates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ready")

	deleted := NewFile(path, FileDeleted).Check(t.Context())
	if deleted.Status != CheckSatisfied {
		t.Fatalf("deleted Satisfied = false, err = %v", deleted.Err)
	}

	if err := os.WriteFile(path, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists := NewFile(path, FileExists).Check(t.Context())
	if exists.Status != CheckSatisfied {
		t.Fatalf("exists Satisfied = false, err = %v", exists.Err)
	}

	containsCond := NewFile(path, FileExists)
	containsCond.Contains = "ready"
	contains := containsCond.Check(t.Context())
	if contains.Status != CheckSatisfied {
		t.Fatalf("contains Satisfied = false, err = %v", contains.Err)
	}
}

func TestFileConditionNonEmptyWaitsForContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	result := NewFile(path, FileNonEmpty).Check(t.Context())
	if result.Status == CheckSatisfied {
		t.Fatal("Satisfied = true, want false")
	}
}
