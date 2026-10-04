package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBackupPathAcceptsOnlyFileName(t *testing.T) {
	base := t.TempDir()
	manager := NewManager(nil, filepath.Join(base, "velora.db"))

	got, err := manager.resolveBackupPath(" backup.db ")
	if err != nil {
		t.Fatalf("resolve backup file name: %v", err)
	}
	if want := filepath.Join(base, "backup.db"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}

	for _, input := range []string{"", ".", "..", "../backup.db", "subdir/backup.db", `/tmp/backup.db`, `subdir\backup.db`} {
		t.Run(input, func(t *testing.T) {
			if _, err := manager.resolveBackupPath(input); err == nil {
				t.Fatalf("accepted unsafe backup path %q", input)
			}
		})
	}
}

func TestVerifyBackupRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.db")
	if err := os.WriteFile(target, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "backup.db")); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(nil, filepath.Join(base, "velora.db"))
	result, err := manager.VerifyBackup("backup.db")
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.Error != "backup path must refer to a regular file" {
		t.Fatalf("unexpected verification result: %+v", result)
	}
}
