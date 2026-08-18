package app

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"immich-go/internal/ml"
	"immich-go/internal/video"
)

// jobState tracks the progress of a running (or finished) job queue. Immich
// reports counts as {active, completed, failed, delayed} but this scaffold
// only needs active/total to drive a progress bar, so we keep the full shape
// for compatibility and fill active/completed.
type jobState struct {
	mu         sync.Mutex
	active     int
	total      int
	completed  int
	failed     int
	running    bool
	startedAt  time.Time
	finishedAt time.Time
	lastError  string
}

func (s *jobState) begin(total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total = total
	s.active = total
	s.completed = 0
	s.failed = 0
	s.running = true
	s.startedAt = time.Now()
	s.lastError = ""
}

func (s *jobState) tick(ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	s.completed++
	if !ok {
		s.failed++
		if err != nil {
			s.lastError = err.Error()
		}
	}
	if s.active <= 0 {
		s.running = false
		s.finishedAt = time.Now()
	}
}

func (s *jobState) snapshot() gin.H {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gin.H{
		"active":    s.active,
		"completed": s.completed,
		"failed":    s.failed,
		"delayed":   0,
		"total":     s.total,
		"running":   s.running,
		"startedAt": s.startedAt.UTC().Format(time.RFC3339),
		"finishedAt": func() string {
			if s.finishedAt.IsZero() {
				return ""
			}
			return s.finishedAt.UTC().Format(time.RFC3339)
		}(),
		"lastError": s.lastError,
	}
}

// jobSpec describes the work for a single job id.
type jobSpec struct {
	// supported marks jobs this server can actually run. AI jobs (object /
	// facial / smart search) are reported for compatibility but not executed.
	supported bool
	// items returns the set of assets (ids + on-disk paths) the job should
	// process. Returning an empty slice means "nothing to do" (a no-op success).
	items func(a *App) ([]jobItem, error)
	// run processes one item. ok=false marks the item failed (counted but not
	// fatal — the job continues with the next item).
	run func(a *App, it jobItem) (ok bool, err error)
}

type jobItem struct {
	ID   string
	Path string
	Type string // IMAGE | VIDEO
}

// jobRegistry maps immich job ids to their implementation. Jobs not listed
// here are unknown and rejected with 400.
func (a *App) jobRegistry() map[string]jobSpec {
	return map[string]jobSpec{
		"thumbnailGeneration": {
			supported: true,
			items: func(a *App) ([]jobItem, error) {
				var assets []Asset
				if err := a.store.DB.Where("has_thumbnail = ? AND is_trash = ?", false, false).Find(&assets).Error; err != nil {
					return nil, err
				}
				out := make([]jobItem, 0, len(assets))
				for _, x := range assets {
					out = append(out, jobItem{ID: x.ID, Path: x.OriginalPath, Type: x.Type})
				}
				return out, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				if _, err := os.Stat(it.Path); err != nil {
					return false, err
				}
				res, err := a.processMedia(it.Path, it.ID, "", it.Type, time.Time{})
				if err != nil {
					return false, err
				}
				thumbPath := ""
				if res.thumbBytes != nil {
					thumbPath = filepath.Join(a.cfg.ResourceDir, "thumbnail", it.ID+".jpg")
				}
				upd := map[string]any{"has_thumbnail": thumbPath != "", "resize_path": thumbPath, "updated_at": time.Now().UTC()}
				if err := a.store.DB.Model(&Asset{}).Where("id = ?", it.ID).Updates(upd).Error; err != nil {
					return false, err
				}
				return thumbPath != "", nil
			},
		},
		"metadataExtraction": {
			supported: true,
			items: func(a *App) ([]jobItem, error) {
				// assets without an exif row, or with an empty exif row.
				var assets []Asset
				if err := a.store.DB.Where("exif_id = ? AND is_trash = ?", "", false).Find(&assets).Error; err != nil {
					return nil, err
				}
				out := make([]jobItem, 0, len(assets))
				for _, x := range assets {
					out = append(out, jobItem{ID: x.ID, Path: x.OriginalPath, Type: x.Type})
				}
				return out, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				if it.Type != "IMAGE" {
					return true, nil // only images carry EXIF in the scaffold
				}
				if _, err := os.Stat(it.Path); err != nil {
					return false, err
				}
				res, err := a.processMedia(it.Path, it.ID, "", it.Type, time.Time{})
				if err != nil {
					return false, err
				}
				if res.exif == nil {
					return true, nil
				}
				// upsert exif row keyed by asset id
				var existing Exif
				if err := a.store.DB.Where("asset_id = ?", it.ID).First(&existing).Error; err != nil {
					res.exif.ID = newUUID()
					res.exif.AssetID = it.ID
					if cerr := a.store.DB.Create(res.exif).Error; cerr != nil {
						return false, cerr
					}
					a.store.DB.Model(&Asset{}).Where("id = ?", it.ID).Update("exif_id", res.exif.ID)
				} else {
					a.store.DB.Model(&Exif{}).Where("id = ?", existing.ID).Updates(map[string]any{
						"make":               res.exif.Make,
						"model":              res.exif.Model,
						"date_time_original": res.exif.DateTimeOriginal,
						"latitude":           res.exif.Latitude,
						"longitude":          res.exif.Longitude,
						"orientation":        res.exif.Orientation,
					})
				}
				return true, nil
			},
		},
		"videoConversion": {
			supported: true,
			items: func(a *App) ([]jobItem, error) {
				var assets []Asset
				if err := a.store.DB.Where("type = ? AND encoded_video_path = ? AND is_trash = ?", "VIDEO", "", false).Find(&assets).Error; err != nil {
					return nil, err
				}
				out := make([]jobItem, 0, len(assets))
				for _, x := range assets {
					out = append(out, jobItem{ID: x.ID, Path: x.OriginalPath, Type: "VIDEO"})
				}
				return out, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				raw, err := os.ReadFile(it.Path)
				if err != nil {
					return false, err
				}
				out, terr := a.video.Transcode(raw, video.TranscodeOptions{
					Format:     "mp4",
					VideoCodec: "h264",
					AudioCodec: "copy",
					Preset:     "software",
				})
				if terr != nil || len(out) == 0 {
					return false, terr
				}
				encDir := filepath.Join(a.cfg.ResourceDir, "encoded-video")
				_ = os.MkdirAll(encDir, 0o755)
				dst := filepath.Join(encDir, it.ID+".mp4")
				if werr := os.WriteFile(dst, out, 0o644); werr != nil {
					return false, werr
				}
				a.store.DB.Model(&Asset{}).Where("id = ?", it.ID).Updates(map[string]any{
					"encoded_video_path": dst,
					"updated_at":         time.Now().UTC(),
				})
				return true, nil
			},
		},
		"duplicateDetection": {
			supported: true,
			items: func(a *App) ([]jobItem, error) {
				// compute duplicate groups by checksum; the result is queryable
				// via GET /api/assets/duplicates. The job itself just (re)scans.
				type dup struct {
					Checksum string
					Count    int
				}
				var dups []dup
				if err := a.store.DB.Model(&Asset{}).Select("checksum, count(*) as count").
					Where("is_trash = ?", false).Group("checksum").Having("count > 1").Scan(&dups).Error; err != nil {
					return nil, err
				}
				out := make([]jobItem, 0, len(dups))
				for i, d := range dups {
					out = append(out, jobItem{ID: d.Checksum, Path: "", Type: ""})
					_ = i
				}
				return out, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				// nothing to mutate per group; detection is query-driven.
				return true, nil
			},
		},
		// Periodic trash-expiry cleanup (Immich's "userDeleteCheck" cron).
		// Supported: it permanently deletes assets older than trashDays.
		"trashCleanup": {
			supported: true,
			items: func(a *App) ([]jobItem, error) {
				return []jobItem{{ID: "trash"}}, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				_, err := a.runTrashCleanup()
				return err == nil, err
			},
		},
		"smartSearch": {
			supported: a.ml != nil && len(a.ml.Capabilities()) > 0,
			items: func(a *App) ([]jobItem, error) {
				var assets []Asset
				if err := a.store.DB.Where("type = ? AND is_trash = ? AND id NOT IN (SELECT asset_id FROM asset_mls)", "IMAGE", false).Find(&assets).Error; err != nil {
					return nil, err
				}
				out := make([]jobItem, 0, len(assets))
				for _, asset := range assets {
					out = append(out, jobItem{ID: asset.ID, Path: asset.OriginalPath, Type: asset.Type})
				}
				return out, nil
			},
			run: func(a *App, it jobItem) (bool, error) {
				data, err := os.ReadFile(it.Path)
				if err != nil {
					return false, err
				}
				result, err := a.ml.Infer(ml.Request{Capability: ml.CapabilityImageDescription, Data: data, MIME: mimeByExt(it.Path), Name: filepath.Base(it.Path), Language: a.cfg.OCRLanguage})
				if err != nil {
					return false, err
				}
				labels, _ := json.Marshal(result.Labels)
				now := time.Now().UTC()
				return true, a.store.DB.Save(&AssetML{AssetID: it.ID, Description: result.Text, LabelsJSON: string(labels), CreatedAt: now, UpdatedAt: now}).Error
			},
		},
		// The following capabilities require separate ML adapters.
		"objectDetection":          {supported: false},
		"facialRecognition":        {supported: false},
		"storageTemplateMigration": {supported: false},
		"tagCopy":                  {supported: false},
		"tagImage":                 {supported: false},
	}
}

// jobWorkers bounds concurrent media processing across all jobs.
const jobWorkers = 4

// handleJobCommand starts (or force-starts) a job. Immich's body may carry
// {force:true}; we always (re)start if not already running.
func (a *App) handleJobCommand(c *gin.Context) {
	id := c.Param("id")
	reg := a.jobRegistry()
	spec, ok := reg[id]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "unknown job: " + id, "statusCode": 400})
		return
	}
	if !spec.supported {
		c.JSON(http.StatusOK, gin.H{
			"jobId":       id,
			"started":     false,
			"unsupported": true,
			"message":     "job requires an ML/AI backend not bundled with immich-go; skipped",
		})
		return
	}

	st := a.jobStateFor(id)
	st.mu.Lock()
	alreadyRunning := st.running
	st.mu.Unlock()
	if alreadyRunning {
		c.JSON(http.StatusOK, gin.H{"jobId": id, "started": false, "alreadyRunning": true})
		return
	}

	items, err := spec.items(a)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	if len(items) == 0 {
		st.begin(0)
		st.mu.Lock()
		st.running = false
		st.finishedAt = time.Now()
		st.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"jobId": id, "started": true, "total": 0, "message": "nothing to process"})
		return
	}

	st.begin(len(items))
	go a.runJob(id, spec, items)
	c.JSON(http.StatusOK, gin.H{"jobId": id, "started": true, "total": len(items)})
}

// runJob executes the items with a bounded worker pool, updating progress.
func (a *App) runJob(id string, spec jobSpec, items []jobItem) {
	st := a.jobStateFor(id)
	sem := make(chan struct{}, jobWorkers)
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it jobItem) {
			defer wg.Done()
			defer func() { <-sem }()
			var ok bool
			var err error
			for attempt := 1; attempt <= 3; attempt++ {
				ok, err = spec.run(a, it)
				if ok || err == nil {
					break
				}
				if attempt < 3 {
					time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
				}
			}
			st.tick(ok, err)
		}(it)
	}
	wg.Wait()
	log.Printf("[jobs] %s finished: %d processed", id, len(items))
}

// handleJobStatus returns live progress for a job id.
func (a *App) handleJobStatus(c *gin.Context) {
	id := c.Param("id")
	st, ok := a.jobStates.Load(id)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"jobId": id, "active": 0, "completed": 0, "total": 0, "running": false})
		return
	}
	c.JSON(http.StatusOK, st.(*jobState).snapshot())
}

// handleJobsList returns the queue status for every known job id.
func (a *App) handleJobsList(c *gin.Context) {
	reg := a.jobRegistry()
	out := gin.H{}
	for id := range reg {
		if st, ok := a.jobStates.Load(id); ok {
			out[id] = st.(*jobState).snapshot()
		} else {
			out[id] = gin.H{"active": 0, "completed": 0, "total": 0, "running": false}
		}
	}
	c.JSON(http.StatusOK, out)
}

// ensure jobStateFor returns (creating if needed) the per-job state.
func (a *App) jobStateFor(id string) *jobState {
	st, _ := a.jobStates.LoadOrStore(id, &jobState{})
	return st.(*jobState)
}

// unused helper kept to avoid import churn if json is needed for debugging.
var _ = json.Marshal
