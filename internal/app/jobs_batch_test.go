package app

import (
	"path/filepath"
	"sync"
	"testing"
)

// S2: runJob processes items in chunks of jobBatchSize, honoring queue pause
// between batches.

func TestRunJobBatchPauseStopsBetweenBatches(t *testing.T) {
	app := newTestApp(t)
	old := jobBatchSize
	jobBatchSize = 2
	defer func() { jobBatchSize = old }()

	spec := app.jobRegistry()["thumbnailGeneration"]
	items := make([]jobItem, 6)
	for i := range items {
		items[i] = jobItem{ID: string(rune('a' + i)), Path: filepath.Join(t.TempDir(), "x.jpg"), Type: "IMAGE"}
	}
	st := app.jobStateFor("thumbnailGeneration")
	st.begin(len(items))

	// Pause immediately: the first batch boundary check must stop the run.
	app.queuePaused.Store("thumbnailGeneration", true)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.runJob("thumbnailGeneration", spec, items)
	}()
	wg.Wait()

	st.mu.Lock()
	processed := st.completed + st.failed
	running := st.running
	st.mu.Unlock()
	if running {
		t.Fatal("job still marked running after pause")
	}
	if processed != 0 {
		t.Fatalf("pause honored too late: processed=%d want 0", processed)
	}
}

func TestRunJobProcessesAllItemsUnpaused(t *testing.T) {
	app := newTestApp(t)
	old := jobBatchSize
	jobBatchSize = 2
	defer func() { jobBatchSize = old }()

	// Use duplicateDetection-style no-op items to count ticks deterministically.
	spec := app.jobRegistry()["duplicateDetection"]
	items := make([]jobItem, 5)
	for i := range items {
		items[i] = jobItem{ID: string(rune('a' + i))}
	}
	st := app.jobStateFor("duplicateDetection")
	st.begin(len(items))
	app.runJob("duplicateDetection", spec, items)

	st.mu.Lock()
	completed := st.completed
	running := st.running
	st.mu.Unlock()
	if completed != 5 || running {
		t.Fatalf("completed=%d running=%v, want 5/false", completed, running)
	}
}

func TestRunJobConcurrentSafety(t *testing.T) {
	app := newTestApp(t)
	spec := app.jobRegistry()["duplicateDetection"]
	items := make([]jobItem, 120)
	for i := range items {
		items[i] = jobItem{ID: "x"}
	}
	old := jobBatchSize
	jobBatchSize = 7
	defer func() { jobBatchSize = old }()
	st := app.jobStateFor("duplicateDetection")
	st.begin(len(items))
	// -race builds will catch unsynchronized state access here.
	app.runJob("duplicateDetection", spec, items)
	st.mu.Lock()
	done := st.completed == len(items)
	st.mu.Unlock()
	if !done {
		t.Fatal("not all items ticked")
	}
	_ = sync.Once{}
}
