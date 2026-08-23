package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S4: createDatabaseBackup writes a consistent snapshot and prunes old ones.

func TestCreateDatabaseBackupAndPrune(t *testing.T) {
	app := newTestApp(t)

	p1, err := app.createDatabaseBackup()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	info, err := os.Stat(p1)
	if err != nil || info.Size() == 0 {
		t.Fatalf("snapshot missing/empty: %v", err)
	}
	if !strings.Contains(filepath.Base(p1), "immich-") {
		t.Fatalf("unexpected name: %s", p1)
	}

	// Create backups beyond the keep count; verify pruning.
	old := backupKeepCount
	defer func() { backupKeepCount = old }()
	for i := 0; i < 5; i++ {
		if _, err := app.createDatabaseBackup(); err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
	}
	entries, _ := os.ReadDir(app.backupDir())
	if len(entries) != backupKeepCount {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("backups on disk = %d (%v), want %d", len(entries), names, backupKeepCount)
	}
}

func TestBackupJobEndToEnd(t *testing.T) {
	app := newTestApp(t)
	spec := app.jobRegistry()["backupDatabase"]
	if !spec.supported {
		t.Fatal("backupDatabase must be supported")
	}
	items, _ := spec.items(app, false)
	if len(items) != 1 {
		t.Fatal("expected single sentinel item")
	}
	ok, err := spec.run(app, items[0])
	if !ok || err != nil {
		t.Fatalf("run: ok=%v err=%v", ok, err)
	}
	entries, _ := os.ReadDir(app.backupDir())
	if len(entries) == 0 {
		t.Fatal("no backup file created by job run")
	}
}
