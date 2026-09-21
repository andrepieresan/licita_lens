package observability

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupStatusReadsRFC3339Timestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last-success")
	want := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	if err := os.WriteFile(path, []byte(want.Format(time.RFC3339)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := BackupStatus(path)
	if err != nil || !got.Equal(want) {
		t.Fatalf("backup status: %v %v", got, err)
	}
}
