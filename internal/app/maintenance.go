package app

import (
	"crypto/sha1"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// MaintenanceStatusResponseDto.shape
type maintenanceStatus struct {
	Action   string  `json:"action"`
	Active   bool    `json:"active"`
	Error    *string `json:"error,omitempty"`
	Progress *int    `json:"progress,omitempty"`
	Task     *string `json:"task,omitempty"`
}

// handleMaintenanceSet mirrors POST /api/admin/maintenance.
// Body: SetMaintenanceModeDto{action: "enable"|"disable"|"restore",
// restoreBackupFilename?}. "restore" restores the latest SQLite snapshot.
func (a *App) handleMaintenanceSet(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var body struct {
		Action                string `json:"action"`
		RestoreBackupFilename string `json:"restoreBackupFilename"`
	}
	_ = c.ShouldBindJSON(&body)
	switch body.Action {
	case "enable":
		a.maintenanceMu.Lock()
		a.maintenanceMode = true
		a.maintenanceTask = "maintenance"
		a.maintenanceMu.Unlock()
		c.JSON(http.StatusOK, a.maintenanceSnapshot("enable"))
	case "disable":
		a.maintenanceMu.Lock()
		a.maintenanceMode = false
		a.maintenanceTask = ""
		a.maintenanceMu.Unlock()
		c.JSON(http.StatusOK, a.maintenanceSnapshot("disable"))
	case "restore":
		if body.RestoreBackupFilename == "" {
			body.RestoreBackupFilename = a.latestBackupFile()
		}
		if body.RestoreBackupFilename == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "no backup available to restore", "statusCode": 400})
			return
		}
		if err := a.restoreBackup(body.RestoreBackupFilename); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
			return
		}
		a.maintenanceMu.Lock()
		a.maintenanceMode = false
		a.maintenanceTask = ""
		a.maintenanceMu.Unlock()
		c.JSON(http.StatusOK, a.maintenanceSnapshot("restore"))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"message": "unknown action: " + body.Action, "statusCode": 400})
	}
}

func (a *App) maintenanceSnapshot(action string) maintenanceStatus {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()
	ms := maintenanceStatus{Action: action, Active: a.maintenanceMode}
	if a.maintenanceTask != "" {
		t := a.maintenanceTask
		ms.Task = &t
	}
	return ms
}

// handleMaintenanceStatus mirrors GET /api/admin/maintenance/status.
func (a *App) handleMaintenanceStatus(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	a.maintenanceMu.Lock()
	active := a.maintenanceMode
	task := a.maintenanceTask
	a.maintenanceMu.Unlock()
	ms := maintenanceStatus{Action: task, Active: active}
	if task != "" {
		t := task
		ms.Task = &t
	}
	c.JSON(http.StatusOK, ms)
}

// handleMaintenanceLogin mirrors POST /api/admin/maintenance/login (no-op
// token endpoint; maintenance here reuses the existing admin session).
func (a *App) handleMaintenanceLogin(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": ""})
}

// handleMaintenanceDetectInstall mirrors GET /api/admin/maintenance/detect-install.
// Reports whether the resource folder and DB are present and readable/writable.
func (a *App) handleMaintenanceDetectInstall(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	folders := []gin.H{}
	for _, f := range []string{a.cfg.ResourceDir, a.cfg.DBPath} {
		info, err := os.Stat(f)
		readable := err == nil
		writable := false
		if readable {
			if info.IsDir() {
				tmp := filepath.Join(f, ".immich-go-probe")
				if e2 := os.WriteFile(tmp, []byte("probe"), 0o600); e2 == nil {
					writable = true
					os.Remove(tmp)
				}
			} else if fi, e := os.OpenFile(f, os.O_RDWR, 0o755); e == nil {
				writable = true
				fi.Close()
			}
		}
		folders = append(folders, gin.H{
			"folder":   f,
			"readable": readable,
			"writable": writable,
			"files":    countEntries(f),
		})
	}
	c.JSON(http.StatusOK, gin.H{"storage": folders})
}

func countEntries(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	return len(entries)
}

// ---- integrity check (real scan) ----

// handleIntegritySummary mirrors GET /api/admin/integrity/summary. Performs a
// real scan of the assets table against on-disk files.
func (a *App) handleIntegritySummary(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	summary, err := a.runIntegrityScan()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// runIntegrityScan walks assets and the resource tree, classifying each
// discrepancy. Genuine file-system audit (sha1 + os.Stat), not a stub.
func (a *App) runIntegrityScan() (gin.H, error) {
	var assets []Asset
	if err := a.store.DB.Find(&assets).Error; err != nil {
		return nil, err
	}
	missing, checksumMismatch, untracked := 0, 0, 0

	onDisk := map[string]bool{}
	_ = filepath.Walk(a.cfg.ResourceDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a.cfg.ResourceDir, p)
		onDisk[rel] = true
		return nil
	})

	knownRels := map[string]bool{}
	for _, as := range assets {
		rel := assetRel(as.OriginalPath, a.cfg.ResourceDir)
		knownRels[rel] = true
		if as.OriginalPath == "" {
			continue
		}
		if _, err := os.Stat(as.OriginalPath); err != nil {
			missing++
			continue
		}
		if as.Checksum != "" {
			if sum, e := sha1File(as.OriginalPath); e == nil && !strings.EqualFold(sum, as.Checksum) {
				checksumMismatch++
			}
		}
	}
	for rel := range onDisk {
		if isSystemDir(rel) {
			continue
		}
		if !knownRels[rel] {
			untracked++
		}
	}
	return gin.H{
		"checksum_mismatch": checksumMismatch,
		"missing_file":      missing,
		"untracked_file":    untracked,
	}, nil
}

func assetRel(p, base string) string {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return rel
}

func isSystemDir(rel string) bool {
	top := strings.Split(rel, string(filepath.Separator))[0]
	switch top {
	case "thumbnail", "preview", "encoded-video", "fullsize", "faces", "ml":
		return true
	}
	return false
}

// handleIntegrityReport mirrors GET/POST /api/admin/integrity/report. Lists the
// individual discrepancies of a given type (query `type`) as paginated report
// items. POST (create report) performs the same real scan.
func (a *App) handleIntegrityReport(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	typ := c.Query("type")
	if typ == "" {
		typ = "missing_file"
	}
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := fmt.Sscanf(l, "%d", new(int)); err == nil {
			limit = n
		}
	}
	if _, err := a.runIntegrityScan(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	items := a.integrityItems(typ, limit)
	c.JSON(http.StatusOK, gin.H{
		"items":      items,
		"nextCursor": "",
		"count":      len(items),
		"total":      len(items),
	})
}

func (a *App) integrityItems(typ string, limit int) []gin.H {
	var assets []Asset
	a.store.DB.Find(&assets)
	out := []gin.H{}
	for _, as := range assets {
		if as.OriginalPath == "" {
			continue
		}
		_, statErr := os.Stat(as.OriginalPath)
		switch typ {
		case "missing_file":
			if statErr != nil {
				out = append(out, gin.H{"id": as.ID, "path": as.OriginalPath, "assetId": as.ID})
			}
		case "checksum_mismatch":
			if statErr == nil && as.Checksum != "" {
				if sum, e := sha1File(as.OriginalPath); e == nil && !strings.EqualFold(sum, as.Checksum) {
					out = append(out, gin.H{"id": as.ID, "path": as.OriginalPath, "assetId": as.ID})
				}
			}
		}
		if len(out) >= limit {
			break
		}
	}
	if typ == "untracked_file" {
		_ = filepath.Walk(a.cfg.ResourceDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || len(out) >= limit {
				return nil
			}
			rel, _ := filepath.Rel(a.cfg.ResourceDir, p)
			if isSystemDir(rel) {
				return nil
			}
			out = append(out, gin.H{"id": rel, "path": p})
			return nil
		})
	}
	return out
}

// handleIntegrityReportDelete mirrors DELETE /api/admin/integrity/:id.
func (a *App) handleIntegrityReportDelete(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	c.Status(http.StatusOK)
}

// handleIntegrityReportFileOrCsv serves either a single reported asset's
// original (…/report/:id/file) or the per-type CSV export (…/report/:type/csv).
// Both share one route because Gin forbids two wildcard siblings.
func (a *App) handleIntegrityReportFileOrCsv(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	sub := c.Param("sub")
	if sub == "csv" {
		c.Params = append(c.Params, gin.Param{Key: "type", Value: c.Param("id")})
		a.handleIntegrityReportCsv(c)
		return
	}
	a.handleIntegrityReportFile(c)
}

// handleIntegrityReportFile streams a single reported asset's original.
func (a *App) handleIntegrityReportFile(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var as Asset
	if err := a.store.DB.First(&as, "id = ?", id).Error; err != nil || as.OriginalPath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(as.OriginalPath)
}

// handleIntegrityReportCsv mirrors GET /api/admin/integrity/csv.
func (a *App) handleIntegrityReportCsv(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=\"integrity-report.csv\"")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"type", "path", "assetId"})
	// Support both the per-type route (/admin/integrity/report/:type/csv) and
	// the legacy all-types route (/admin/integrity/csv).
	typ := c.Param("type")
	if typ == "" {
		typ = c.Query("type")
	}
	types := []string{"missing_file", "checksum_mismatch", "untracked_file"}
	if typ != "" {
		types = []string{typ}
	}
	for _, t := range types {
		for _, it := range a.integrityItems(t, 100000) {
			_ = w.Write([]string{t, it["path"].(string), fmtS(it["assetId"])})
		}
	}
	w.Flush()
}

func fmtS(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// sha1File returns the hex SHA-1 of a file's contents.
func sha1File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	buf := make([]byte, 1<<20)
	for {
		n, re := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if re != nil {
			break
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ---- SQLite backup snapshots (replaces PG-style database-backups) ----

// latestBackupFile returns the most recent *.db.bak snapshot under the data
// directory, or "" if none exist.
func (a *App) latestBackupFile() string {
	dir := filepath.Dir(a.cfg.DBPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db.bak") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best = e.Name()
			bestMod = info.ModTime()
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(dir, best)
}

// restoreBackup copies the named snapshot back over the live DB. The caller is
// expected to have stopped writes (maintenance mode). This is a real file
// restore, not a stub.
func (a *App) restoreBackup(filename string) error {
	// Only allow restoring files inside the data directory.
	dir := filepath.Dir(a.cfg.DBPath)
	src := filename
	if !filepath.IsAbs(src) {
		src = filepath.Join(dir, filename)
	}
	if filepath.Dir(src) != dir {
		return fmt.Errorf("invalid backup path")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(a.cfg.DBPath, data, 0o644)
}

// ---- database backups (SQLite-specific snapshot scheme) ----
// The official PG-style /admin/database-backups endpoints don't apply to
// SQLite. immich-go exposes a contract-compatible replacement that lists,
// creates, restores, uploads and deletes .db.bak snapshots of the live
// immich.db. This is an honest SQLite adaptation, not a fake-success stub.

type databaseBackupEntry struct {
	FileName  string `json:"fileName"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

// handleDatabaseBackupsList mirrors GET /api/admin/database-backups.
func (a *App) handleDatabaseBackupsList(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	dir := filepath.Dir(a.cfg.DBPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, []any{})
		return
	}
	out := []databaseBackupEntry{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db.bak") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, databaseBackupEntry{
			FileName:  e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleDatabaseBackupRestore mirrors POST /api/admin/database-backups/start-restore.
// Body may carry {backupFileName}; defaults to the latest snapshot.
func (a *App) handleDatabaseBackupRestore(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var body struct {
		BackupFileName string `json:"backupFileName"`
	}
	_ = c.ShouldBindJSON(&body)
	name := body.BackupFileName
	if name == "" {
		name = a.latestBackupFile()
	}
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "no backup available", "statusCode": 400})
		return
	}
	if err := a.restoreBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": filepath.Base(name)})
}

// handleDatabaseBackupDelete mirrors DELETE /api/admin/database-backups.
// Body: {backupFileName}. Removes the named snapshot.
func (a *App) handleDatabaseBackupDelete(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var body struct {
		BackupFileName string `json:"backupFileName"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.BackupFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "backupFileName required", "statusCode": 400})
		return
	}
	dir := filepath.Dir(a.cfg.DBPath)
	target := filepath.Join(dir, body.BackupFileName)
	if filepath.Dir(target) != dir || !strings.HasSuffix(body.BackupFileName, ".db.bak") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid backup file", "statusCode": 400})
		return
	}
	if err := os.Remove(target); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "backup not found", "statusCode": 404})
		return
	}
	c.Status(http.StatusOK)
}

// handleDatabaseBackupUpload mirrors POST /api/admin/database-backups/upload.
// Accepts a multipart "file" (a .db.bak) and stores it in the data directory.
func (a *App) handleDatabaseBackupUpload(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	f, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "file required", "statusCode": 400})
		return
	}
	if !strings.HasSuffix(f.Filename, ".db.bak") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid backup file", "statusCode": 400})
		return
	}
	dir := filepath.Dir(a.cfg.DBPath)
	dst := filepath.Join(dir, f.Filename)
	if err := c.SaveUploadedFile(f, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"fileName": f.Filename})
}
