package app

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

// queueNames is the full set of Immich queue names (mirrors the official
// QueueName enum). Every name is reported by GET /queues so the admin queue
// monitor renders all cards; immich-go only actively processes the subset it
// supports, but the remaining queues are still real (idle) and pausable.
var queueNames = []string{
	"thumbnailGeneration",
	"metadataExtraction",
	"videoConversion",
	"faceDetection",
	"facialRecognition",
	"smartSearch",
	"duplicateDetection",
	"backgroundTask",
	"storageTemplateMigration",
	"migration",
	"search",
	"sidecar",
	"library",
	"notifications",
	"backupDatabase",
	"ocr",
	"workflow",
	"integrityCheck",
	"editor",
}

// queueStats assembles the QueueStatisticsDto for one queue name. It reuses
// the live job progress tracked in jobStates and overlays the paused flag.
// All six fields are required by the official DTO — omitting `waiting` made
// the admin jobs page render NaN (it computes waiting+paused+delayed).
func (a *App) queueStats(name string) gin.H {
	paused := false
	if v, ok := a.queuePaused.Load(name); ok && v.(bool) {
		paused = true
	}
	running := a.queueRunning(name)
	st := gin.H{
		"active":    0,
		"completed": 0,
		"failed":    0,
		"delayed":   0,
		"paused":    0,
		"waiting":   0,
	}
	var wait int
	if v, ok := a.jobStates.Load(name); ok {
		snap := v.(*jobState).snapshot()
		active, _ := snap["active"].(int)
		completed, _ := snap["completed"].(int)
		failed, _ := snap["failed"].(int)
		wait = active
		st["active"] = active
		st["completed"] = completed
		st["failed"] = failed
		st["delayed"] = snap["delayed"]
	}
	if !running {
		wait = 0
	}
	st["waiting"] = wait
	if paused {
		st["paused"] = 1
	}
	return st
}

// queueRunning reports whether the named queue has work in flight.
func (a *App) queueRunning(name string) bool {
	v, ok := a.jobStates.Load(name)
	if !ok {
		return false
	}
	s := v.(*jobState)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// handleQueuesList mirrors GET /api/queues. Returns the full list of queues
// with live statistics and paused state.
func (a *App) handleQueuesList(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	out := make([]gin.H, 0, len(queueNames))
	for _, name := range queueNames {
		paused := false
		if v, ok := a.queuePaused.Load(name); ok {
			paused = v.(bool)
		}
		out = append(out, gin.H{
			"name":     name,
			"isPaused": paused,
			"statistics": a.queueStats(name),
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleQueueGet mirrors GET /api/queues/:name.
func (a *App) handleQueueGet(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	name := c.Param("name")
	paused := false
	if v, ok := a.queuePaused.Load(name); ok {
		paused = v.(bool)
	}
	c.JSON(http.StatusOK, gin.H{
		"name":     name,
		"isPaused": paused,
		"statistics": a.queueStats(name),
	})
}

// handleQueueUpdate mirrors PUT /api/queues/:name. Honors isPaused to pause or
// resume the in-memory queue.
func (a *App) handleQueueUpdate(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	name := c.Param("name")
	var body struct {
		IsPaused *bool `json:"isPaused"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.IsPaused != nil {
		a.queuePaused.Store(name, *body.IsPaused)
	}
	paused := false
	if v, ok := a.queuePaused.Load(name); ok {
		paused = v.(bool)
	}
	c.JSON(http.StatusOK, gin.H{
		"name":     name,
		"isPaused": paused,
		"statistics": a.queueStats(name),
	})
}

// handleQueueJobs mirrors GET /api/queues/:name/jobs?status=. immich-go's
// in-memory bus does not retain individual job records, so this returns the
// live (active) or completed jobs derived from the queue's progress snapshot.
// This is an honest reflection of the in-memory model, not a fake success.
func (a *App) handleQueueJobs(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	name := c.Param("name")
	status := c.QueryArray("status")
	if len(status) == 0 {
		status = []string{"active", "completed", "failed", "delayed"}
	}
	snap := gin.H{
		"active":    0,
		"completed": 0,
		"failed":    0,
		"delayed":   0,
	}
	if v, ok := a.jobStates.Load(name); ok {
		s := v.(*jobState).snapshot()
		snap["active"] = s["active"]
		snap["completed"] = s["completed"]
		snap["failed"] = s["failed"]
		snap["delayed"] = s["delayed"]
	}
	jobs := []gin.H{}
	want := map[string]bool{}
	for _, st := range status {
		want[st] = true
	}
	if want["active"] && snap["active"].(int) > 0 {
		jobs = append(jobs, gin.H{
			"id":        name + ":active",
			"name":      name,
			"timestamp": 0,
			"data":      gin.H{},
		})
	}
	if want["completed"] && snap["completed"].(int) > 0 {
		jobs = append(jobs, gin.H{
			"id":        name + ":completed",
			"name":      name,
			"timestamp": 0,
			"data":      gin.H{},
		})
	}
	if want["failed"] && snap["failed"].(int) > 0 {
		jobs = append(jobs, gin.H{
			"id":        name + ":failed",
			"name":      name,
			"timestamp": 0,
			"data":      gin.H{},
		})
	}
	c.JSON(http.StatusOK, jobs)
}

// handleQueueEmpty mirrors DELETE /api/queues/:name/jobs. Clears failed jobs
// (and resets the queue's tracked progress). With the in-memory bus this
// resets the tracked state for the queue.
func (a *App) handleQueueEmpty(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	name := c.Param("name")
	var body struct {
		Failed bool `json:"failed"`
	}
	_ = c.ShouldBindJSON(&body)
	if st, ok := a.jobStates.Load(name); ok {
		s := st.(*jobState)
		s.mu.Lock()
		if body.Failed {
			s.failed = 0
		}
		s.completed = 0
		s.active = 0
		s.total = 0
		s.running = false
		s.lastError = ""
		s.mu.Unlock()
	}
	a.queuePaused.Store(name, false)
	c.Status(http.StatusOK)
}

// sortedQueueNames returns the queue names in a stable order (used by /jobs
// legacy mapping).
func sortedQueueNames() []string {
	names := append([]string{}, queueNames...)
	sort.Strings(names)
	return names
}
