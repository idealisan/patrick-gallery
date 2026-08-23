package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// S3: force dispatch must run in chained batches — first batch materialized
// only, cursor advances, upper bound excludes post-trigger assets.

func TestChainedForceProcessesAllBatches(t *testing.T) {
	app := newTestApp(t)
	old := jobBatchSize
	jobBatchSize = 2
	defer func() { jobBatchSize = old }()

	if err := app.store.DB.Create(&User{ID: "owner", Email: "a@b.c", IsAdmin: true}).Error; err != nil {
		t.Fatal(err)
	}
	// 5 assets -> batches of 2,2,1
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		if err := app.store.DB.Create(&Asset{
			ID: id, OwnerID: "owner", Type: "IMAGE",
			OriginalPath: filepath.Join(t.TempDir(), id+".jpg"),
			HasThumbnail: true,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/thumbnailGeneration",
		bytes.NewBufferString(`{"command":"start","force":true}`))
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "thumbnailGeneration"}}
	c.Set(ctxUserID, "owner")
	app.handleJobCommand(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// Wait for chain completion (bounded).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ch, ok := jobChains.Load("thumbnailGeneration")
		if !ok {
			break // chain removed on completion
		}
		_ = ch
		time.Sleep(20 * time.Millisecond)
	}
	st := app.jobStateFor("thumbnailGeneration")
	st.mu.Lock()
	completed := st.completed
	total := st.total
	st.mu.Unlock()
	if completed < 5 {
		t.Fatalf("completed=%d total=%d, want all 5 processed", completed, total)
	}
	if _, still := jobChains.Load("thumbnailGeneration"); still {
		t.Fatal("chain not cleaned up after completion")
	}
}

func TestChainedForceExcludesPostTriggerAssets(t *testing.T) {
	app := newTestApp(t)
	old := jobBatchSize
	jobBatchSize = 1
	defer func() { jobBatchSize = old }()

	if err := app.store.DB.Create(&User{ID: "owner", Email: "a@b.c", IsAdmin: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.store.DB.Create(&Asset{
		ID: "before", OwnerID: "owner", Type: "IMAGE", HasThumbnail: true,
		OriginalPath: filepath.Join(t.TempDir(), "b.jpg"),
	}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/thumbnailGeneration", bytes.NewBufferString(`{"command":"start","force":true}`))
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "thumbnailGeneration"}}
	c.Set(ctxUserID, "owner")
	app.handleJobCommand(c)

	// Insert AFTER trigger: must NOT be part of this chained run.
	if err := app.store.DB.Create(&Asset{
		ID: "zz-after", OwnerID: "owner", Type: "IMAGE", HasThumbnail: true,
		OriginalPath: filepath.Join(t.TempDir(), "z.jpg"),
	}).Error; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := jobChains.Load("thumbnailGeneration"); !ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := app.jobStateFor("thumbnailGeneration")
	st.mu.Lock()
	completed := st.completed
	st.mu.Unlock()
	if completed != 1 {
		t.Fatalf("completed=%d, want exactly 1 (post-trigger asset excluded)", completed)
	}
}
