package observability

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// BackupStatus reads a UTC RFC3339 timestamp written only after the complete
// recovery set (database, raw archive and queue data) has been copied.
func BackupStatus(path string) (time.Time, error) {
	if strings.TrimSpace(path) == "" {
		return time.Time{}, fmt.Errorf("backup status file is not configured")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	value, err := time.Parse(time.RFC3339, strings.TrimSpace(string(body)))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse backup status: %w", err)
	}
	return value.UTC(), nil
}
