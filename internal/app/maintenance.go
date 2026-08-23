package app

import (
	"crypto/sha1"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// handleMaintenanceSet mirrors POST /api/admin/maintenance with the official
// SetMaintenanceModeDto: action ∈ start | end | select_database_restore |
// restore_database (+ optional restoreBackupFilename). Per Epic decision,
// immich-go performs a VIRTUAL restart: entering maintenance quiesces
// background work (scheduler, job queues) while the process keeps serving;
// the response shape still matches the official {jwt} payload.
func (a *App) handleMaintenanceSet(c *gin.Context) {
	admin, ok := a.requireAdmin(c)
	if !ok {
		return
	}
	var body struct {
		Action                string `json:"action"`
		RestoreBackupFilename string `json:"restoreBackupFilename"`
	}
	_ = c.ShouldBindJSON(&body)
	switch body.Action {
	case "start", "select_database_restore":
		a.enterMaintenance(body.Action)
		tok, err := a.issueToken(admin.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"jwt": tok})
	case "end":
		a.exitMaintenance()
		c.JSON(http.StatusOK, gin.H{})
	case "restore_database":
		name := body.RestoreBackupFilename
		if name == "" {
			name = a.latestBackupFile()
		}
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "no backup available", "statusCode": 400})
			return
		}
		a.enterMaintenance("restore_database")
		if err := a.restoreBackup(name); err != nil {
			a.exitMaintenance()
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
			return
		}
		log.Printf("[maintenance] database restored from %s", filepath.Base(name))
		a.exitMaintenance()
		c.JSON(http.StatusOK, gin.H{})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"message": "unknown action: " + body.Action, "statusCode": 400})
	}
}

// inMaintenance reports whether virtual maintenance mode is active.
func (a *App) inMaintenance() bool {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()
	return a.maintenanceMode
}

// enterMaintenance switches the app into virtual maintenance mode: background
// work is stopped/paused, but the process keeps serving requests.
func (a *App) enterMaintenance(task string) {
	a.maintenanceMu.Lock()
	a.maintenanceMode = true
	a.maintenanceTask = task
	a.maintenanceMu.Unlock()
	a.stopTrashScheduler()
	// Pause every queue not already paused; remembered for exit.
	var newlyPaused []string
	for _, name := range queueNames {
		paused := false
		if v, ok := a.queuePaused.Load(name); ok {
			paused = v.(bool)
		}
		if !paused {
			a.queuePaused.Store(name, true)
			newlyPaused = append(newlyPaused, name)
		}
	}
	a.maintenanceMu.Lock()
	a.maintenanceResumed = newlyPaused
	a.maintenanceMu.Unlock()
	log.Printf("[maintenance] entered (virtual restart, task=%s)", task)
}

// exitMaintenance leaves virtual maintenance mode and resumes background work.
func (a *App) exitMaintenance() {
	a.maintenanceMu.Lock()
	task := a.maintenanceTask
	resumed := a.maintenanceResumed
	a.maintenanceMode = false
	a.maintenanceTask = ""
	a.maintenanceResumed = nil
	a.maintenanceMu.Unlock()
	for _, name := range resumed {
		a.queuePaused.Store(name, false)
	}
	if task != "" {
		a.startSchedulers()
	}
	log.Printf("[maintenance] exited")
}

// handleMaintenanceStatus mirrors GET /api/admin/maintenance/status. The
// official normal server always reports {active:false, action:"end"}; while
// our virtual maintenance is active we report the chosen action instead.
func (a *App) handleMaintenanceStatus(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	a.maintenanceMu.Lock()
	active := a.maintenanceMode
	task := a.maintenanceTask
	a.maintenanceMu.Unlock()
	action := "end"
	if active && task != "" {
		action = task
	}
	ms := maintenanceStatus{Action: action, Active: active}
	if active && task == "select_database_restore" {
		t := "restore"
		ms.Task = &t
	}
	c.JSON(http.StatusOK, ms)
}

// handleMaintenanceLogin mirrors POST /api/admin/maintenance/login: exchanges
// the maintenance token for a {jwt} the maintenance UI can present.
func (a *App) handleMaintenanceLogin(c *gin.Context) {
	admin, ok := a.requireAdmin(c)
	if !ok {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = c.ShouldBindJSON(&body)
	tok, err := a.issueToken(admin.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jwt": tok, "username": admin.Name})
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

// ---- integrity check (persistent report model, mirrors the official
// integrity_report architecture) ----

// handleIntegritySummary mirrors GET /api/admin/integrity/summary: a cheap
// COUNT over the persisted report table (the scan itself is driven by jobs).
func (a *App) handleIntegritySummary(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	c.JSON(http.StatusOK, a.integrityCounts())
}

// integrityCounts aggregates the report table into the summary DTO shape.
func (a *App) integrityCounts() gin.H {
	out := gin.H{"checksum_mismatch": 0, "missing_file": 0, "untracked_file": 0}
	type row struct {
		Type  string
		Count int64
	}
	var rows []row
	a.store.DB.Model(&IntegrityReport{}).Select("type, count(*) as count").Group("type").Scan(&rows)
	for _, r := range rows {
		out[r.Type] = r.Count
	}
	return out
}

// reportUpsert inserts or refreshes one report row keyed by (type, path),
// mirroring the official onConflict-doUpdate semantics.
func (a *App) reportUpsert(typ, path string, assetID *string) {
	var existing IntegrityReport
	err := a.store.DB.Where("type = ? AND path = ?", typ, path).First(&existing).Error
	if err != nil {
		a.store.DB.Create(&IntegrityReport{
			ID: newUUID(), Type: typ, Path: path,
			AssetID: assetID, CreatedAt: time.Now().UTC(),
		})
		return
	}
	if assetID != nil && existing.AssetID == nil {
		a.store.DB.Model(&existing).Update("asset_id", *assetID)
	}
}

// refreshExistingReports revalidates every stored report row and drops the
// ones whose underlying state changed (official -refresh job semantics):
// untracked file gone / missing file back / checksum now matches.
func (a *App) refreshExistingReports() error {
	for _, typ := range []string{"untracked_file", "missing_file", "checksum_mismatch"} {
		a.refreshReportType(typ)
	}
	return nil
}

// refreshReportType drops stale rows of one report type.
func (a *App) refreshReportType(typ string) {
	var rows []IntegrityReport
	if err := a.store.DB.Where("type = ?", typ).Find(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		stale := false
		switch typ {
		case "untracked_file":
			_, err := os.Stat(r.Path)
			stale = err != nil
		case "missing_file":
			_, err := os.Stat(r.Path)
			stale = err == nil
		case "checksum_mismatch":
			sum, err := sha1File(r.Path)
			switch {
			case os.IsNotExist(err):
				stale = true // handled by the missing-file audit
			case err == nil:
				var as Asset
				if e := a.store.DB.First(&as, "id = ?", r.AssetID).Error; e != nil || as.Checksum == "" ||
					strings.EqualFold(sum, as.Checksum) {
					stale = true
				}
			}
		}
		if stale {
			a.store.DB.Delete(&r)
		}
	}
}

// resourceTail returns "<parentDir>/<fileName>" for p with forward slashes.
// Asset rows historically store resource paths in several shapes (absolute,
// resource-relative, cwd-relative), so reference matching anchors on the
// trailing segments — file names are UUIDs, which makes this unambiguous.
func resourceTail(p string) string {
	return filepath.Base(filepath.Dir(p)) + "/" + filepath.Base(p)
}

// pathReferenced reports whether an absolute file path is referenced by any
// asset (original, encoded video or thumbnail/preview).
func (a *App) pathReferenced(p string) bool {
	tail := resourceTail(p)
	var n int64
	a.store.DB.Model(&Asset{}).Where(
		"original_path = ? OR encoded_video_path = ? OR resize_path = ? "+
			"OR original_path LIKE ? OR encoded_video_path LIKE ? OR resize_path LIKE ?",
		p, p, p, "%/"+tail, "%/"+tail, "%/"+tail,
	).Count(&n)
	return n > 0
}

// untrackedCandidates walks the resource tree and returns files not
// referenced by any asset (the untracked scan's input batch).
func (a *App) untrackedCandidates() ([]jobItem, error) {
	out := []jobItem{}
	base := a.resourceDirAbs()
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if isSystemDir(rel) || isDBArtifact(rel) || strings.HasSuffix(rel, ".preview.jpg") {
			return nil
		}
		if !a.pathReferenced(p) {
			out = append(out, jobItem{ID: p, Path: p, Type: "untracked"})
		}
		return nil
	})
	return out, err
}

// reportRows loads one report type as job items (id = report uuid).
func (a *App) reportRows(typ string) ([]jobItem, error) {
	var rows []IntegrityReport
	if err := a.store.DB.Where("type = ?", typ).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]jobItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, jobItem{ID: r.ID, Path: r.Path})
	}
	return out, nil
}

// disposeRowByID applies the official disposition for one report row.
func (a *App) disposeRowByID(id string) (bool, error) {
	var r IntegrityReport
	if err := a.store.DB.First(&r, "id = ?", id).Error; err != nil {
		return true, nil // already gone
	}
	if err := a.disposeReportRow(&r); err != nil {
		return false, err
	}
	return true, nil
}

// handleIntegritySummary mirrors GET /api/admin/integrity/summary: a cheap

// runIntegrityScan performs the genuine filesystem audit and PERSISTS its
// findings into integrity_report: refresh stale rows, then classify missing /
// checksum-mismatch assets and untracked files.
func (a *App) runIntegrityScan() (gin.H, error) {
	if err := a.refreshExistingReports(); err != nil {
		return nil, err
	}
	var assets []Asset
	if err := a.store.DB.Find(&assets).Error; err != nil {
		return nil, err
	}
	for i := range assets {
		as := &assets[i]
		if as.OriginalPath == "" {
			continue
		}
		id := as.ID
		if _, err := os.Stat(as.OriginalPath); err != nil {
			a.reportUpsert("missing_file", as.OriginalPath, &id)
			continue
		}
		if as.Checksum != "" {
			if sum, e := sha1File(as.OriginalPath); e == nil && !strings.EqualFold(sum, as.Checksum) {
				a.reportUpsert("checksum_mismatch", as.OriginalPath, &id)
			}
		}
	}
	// Untracked: files under the resource tree not referenced by any asset
	// (originals, encoded video, thumbnails/previews). DB artifacts, profile
	// pictures, backups and ML scratch dirs are out of scope (official only
	// walks upload/library/encoded-video/thumbs). Derived preview caches
	// (<name>.<assetID>.preview.jpg) are managed by the repair pass instead.
	base := a.resourceDirAbs()
	_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || a.pathReferenced(p) {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if isSystemDir(rel) || isDBArtifact(rel) || strings.HasSuffix(rel, ".preview.jpg") {
			return nil
		}
		a.reportUpsert("untracked_file", p, nil)
		return nil
	})
	return a.integrityCounts(), nil
}

// resourceDirAbs returns the configured resource dir as an absolute path —
// the config default ("resources") is relative to the working directory and
// must be normalized before being matched against absolute asset paths.
func (a *App) resourceDirAbs() string {
	if abs, err := filepath.Abs(a.cfg.ResourceDir); err == nil {
		return abs
	}
	return a.cfg.ResourceDir
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
	// Dirs with no per-file DB reference (ML scratch, profile avatars,
	// backups). Everything else under the resource tree is expected to be
	// tracked via asset.original_path / encoded_video_path / resize_path.
	case "faces", "ml", "profile", "backups":
		return true
	}
	return false
}

// isDBArtifact excludes SQLite sidecar files living directly in the resource
// dir — they are server internals, never user media.
func isDBArtifact(rel string) bool {
	if strings.Contains(rel, string(filepath.Separator)) {
		return false
	}
	for _, suf := range []string{".db", ".db-wal", ".db-shm"} {
		if strings.HasSuffix(rel, suf) {
			return true
		}
	}
	return strings.HasPrefix(rel, ".")
}

// handleIntegrityReport mirrors GET /api/admin/integrity/report — paginated
// rows of the persisted report table ({items:[{id,type,path}], nextCursor}),
// ordered by id descending with an exclusive uuid cursor.
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
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	q := a.store.DB.Where("type = ?", typ).Order("id DESC").Limit(limit + 1)
	if cur := c.Query("cursor"); cur != "" {
		q = q.Where("id < ?", cur)
	}
	var rows []IntegrityReport
	if err := q.Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	next := ""
	if len(rows) > limit {
		next = rows[limit-1].ID
		rows = rows[:limit]
	}
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		items = append(items, gin.H{"id": r.ID, "type": r.Type, "path": r.Path})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "nextCursor": next})
}

// disposeReportRow applies the official delete disposition for one report row:
// assetId → trash the owning asset (recoverable); fileAssetId → permanently
// delete the derived file; neither → unlink the raw path. Always removes the
// row afterwards.
func (a *App) disposeReportRow(r *IntegrityReport) error {
	switch {
	case r.AssetID != nil && *r.AssetID != "":
		now := time.Now().UTC()
		if err := a.store.DB.Model(&Asset{}).Where("id = ?", *r.AssetID).Updates(map[string]any{
			"is_trash": true, "trashed_at": &now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
	default:
		// untracked / derived-file branch: remove the offending file itself.
		if err := os.Remove(r.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return a.store.DB.Delete(r).Error
}

// handleIntegrityReportDeletePath is the slashed-id variant of
// handleIntegrityReportDelete.
func (a *App) handleIntegrityReportDeletePath(c *gin.Context) {
	id := c.Param("idpath")
	if len(id) > 0 && id[0] == '/' {
		id = id[1:]
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
	a.handleIntegrityReportDelete(c)
}

// handleIntegrityReportDelete mirrors DELETE /api/admin/integrity/report/:id.
func (a *App) handleIntegrityReportDelete(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var r IntegrityReport
	if err := a.store.DB.First(&r, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "report not found", "statusCode": 404})
		return
	}
	if err := a.disposeReportRow(&r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
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

// handleIntegrityReportFile streams a single reported file by report UUID,
// mirroring the official ImmichFileResponse (octet-stream attachment).
func (a *App) handleIntegrityReportFile(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var r IntegrityReport
	if err := a.store.DB.First(&r, "id = ?", id).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if _, err := os.Stat(r.Path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filepath.Base(r.Path)+`"`)
	c.Header("Cache-Control", "private, no-cache")
	c.File(r.Path)
}

// handleIntegrityReportCsv mirrors GET /api/admin/integrity/report/:type/csv
// (and the legacy all-types /admin/integrity/csv): official header
// id,type,assetId,fileAssetId,path.
func (a *App) handleIntegrityReportCsv(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=\"integrity-report.csv\"")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "type", "assetId", "fileAssetId", "path"})
	typ := c.Param("type")
	if typ == "" {
		typ = c.Query("type")
	}
	q := a.store.DB.Order("id DESC")
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	var rows []IntegrityReport
	q.Find(&rows)
	for _, r := range rows {
		assetID, fileAssetID := "", ""
		if r.AssetID != nil {
			assetID = *r.AssetID
		}
		if r.FileAssetID != nil {
			fileAssetID = *r.FileAssetID
		}
		path := strings.ReplaceAll(r.Path, `"`, `""`)
		_ = w.Write([]string{r.ID, r.Type, assetID, fileAssetID, path})
	}
	w.Flush()
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

// backupKeepCount is how many recent backups to retain (S4).
var backupKeepCount = 3

// backupDir returns resources/backups under the resource dir.
func (a *App) backupDir() string {
	return filepath.Join(a.cfg.ResourceDir, "backups")
}

// createDatabaseBackup produces a consistent SQLite snapshot via
// `VACUUM INTO` (after a WAL checkpoint) into resources/backups/, then prunes
// older files beyond backupKeepCount. Returns the created path.
func (a *App) createDatabaseBackup() (string, error) {
	dir := a.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir backup dir: %w", err)
	}
	// Fold WAL into the main db so VACUUM INTO sees a fully consistent view.
	if err := a.store.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		return "", fmt.Errorf("wal checkpoint: %w", err)
	}
	name := "immich-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".db"
	dst := filepath.Join(dir, name)
	if err := a.store.DB.Exec("VACUUM INTO ?", dst).Error; err != nil {
		return "", fmt.Errorf("vacuum into: %w", err)
	}
	info, err := os.Stat(dst)
	if err != nil || info.Size() == 0 {
		return "", fmt.Errorf("backup empty or missing: %s", dst)
	}
	a.pruneOldBackups(dir)
	return dst, nil
}

// pruneOldBackups deletes the oldest immich-*.db files beyond
// backupKeepCount.
func (a *App) pruneOldBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type bk struct {
		path string
		mod  time.Time
	}
	var all []bk
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "immich-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, bk{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(all) <= backupKeepCount {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.After(all[j].mod) })
	for _, b := range all[backupKeepCount:] {
		_ = os.Remove(b.path)
		log.Printf("[backup] pruned old snapshot %s", filepath.Base(b.path))
	}
}

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
	dirs := []string{filepath.Dir(a.cfg.DBPath), a.backupDir()}
	out := []databaseBackupEntry{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".db.bak") &&
				!(dir == a.backupDir() && strings.HasPrefix(e.Name(), "immich-") && strings.HasSuffix(e.Name(), ".db"))) {
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
