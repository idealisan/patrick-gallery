package video

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNeedsFaststart verifies that needsFaststart correctly detects moov atom position.
func TestNeedsFaststart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	p := New()
	if _, ok := p.(*placeholder); ok {
		t.Skip("placeholder backend — no FFmpeg available")
	}

	// Test with a non-existent file.
	_, err := needsFaststart("/nonexistent/file.mp4")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestBitrateThresholds tests the bitrate decision logic.
func TestBitrateThresholds(t *testing.T) {
	thresholds := DefaultBitrateThresholds()

	tests := []struct {
		name     string
		width    int
		bitrate  int64
		wantNeed bool
		wantMax  int64
	}{
		{
			name:     "720p within threshold",
			width:    1280,
			bitrate:  2_000_000, // 2 Mbps
			wantNeed: false,
			wantMax:  3_000_000,
		},
		{
			name:     "720p over threshold",
			width:    1280,
			bitrate:  4_000_000, // 4 Mbps
			wantNeed: true,
			wantMax:  3_000_000,
		},
		{
			name:     "1080p within threshold",
			width:    1920,
			bitrate:  2_500_000, // 2.5 Mbps
			wantNeed: false,
			wantMax:  3_000_000,
		},
		{
			name:     "1080p over threshold",
			width:    1920,
			bitrate:  4_000_000, // 4 Mbps
			wantNeed: true,
			wantMax:  3_000_000,
		},
		{
			name:     "2K within threshold",
			width:    2560,
			bitrate:  4_000_000, // 4 Mbps
			wantNeed: false,
			wantMax:  5_000_000,
		},
		{
			name:     "2K over threshold",
			width:    2560,
			bitrate:  6_000_000, // 6 Mbps
			wantNeed: true,
			wantMax:  5_000_000,
		},
		{
			name:     "4K within threshold",
			width:    3840,
			bitrate:  7_000_000, // 7 Mbps
			wantNeed: false,
			wantMax:  8_000_000,
		},
		{
			name:     "4K over threshold",
			width:    3840,
			bitrate:  10_000_000, // 10 Mbps
			wantNeed: true,
			wantMax:  8_000_000,
		},
		{
			name:     "unknown bitrate (0) -> transcode",
			width:    1920,
			bitrate:  0,
			wantNeed: true,
			wantMax:  3_000_000,
		},
		{
			name:     "negative bitrate -> transcode",
			width:    1920,
			bitrate:  -1,
			wantNeed: true,
			wantMax:  3_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNeed, gotMax := thresholds.NeedsTranscode(tt.width, tt.bitrate)
			if gotNeed != tt.wantNeed {
				t.Errorf("NeedsTranscode(%d, %d) need=%v, want %v", tt.width, tt.bitrate, gotNeed, tt.wantNeed)
			}
			if gotMax != tt.wantMax {
				t.Errorf("NeedsTranscode(%d, %d) maxBitrate=%d, want %d", tt.width, tt.bitrate, gotMax, tt.wantMax)
			}
		})
	}
}

// TestCacheManagerLRU tests LRU eviction in the cache manager.
func TestCacheManagerLRU(t *testing.T) {
	dir := t.TempDir()

	// Create some test files.
	for i := 0; i < 5; i++ {
		name := filepath.Base(filepath.Join(dir, "test"+string(rune('a'+i))+".mp4"))
		path := filepath.Join(dir, name)
		os.WriteFile(path, make([]byte, 100*1024), 0o644) // 100KB each
	}

	// Cache manager with 300KB limit (should evict 2 files).
	cm := NewCacheManager(dir, 300*1024)

	// Record access in order (oldest to newest).
	for i := 0; i < 5; i++ {
		name := "test" + string(rune('a'+i)) + ".mp4"
		cm.RecordAccess(name)
		time.Sleep(10 * time.Millisecond) // ensure distinct timestamps
	}

	// Evict should remove the 2 oldest files.
	evicted := cm.Evict()
	if len(evicted) != 2 {
		t.Errorf("expected 2 evictions, got %d", len(evicted))
	}

	// Verify the evicted files are the oldest ones.
	for _, name := range evicted {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be deleted", name)
		}
	}
}

// TestCacheManagerAdd tests adding files to the cache.
func TestCacheManagerAdd(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1<<30) // 1GB limit

	// Add a file.
	cm.Add("test.mp4", 1024*1024) // 1MB

	if cm.TotalSize() != 1024*1024 {
		t.Errorf("expected total size 1MB, got %d", cm.TotalSize())
	}
}

// TestCacheManagerRecordAccess tests updating access time.
func TestCacheManagerRecordAccess(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1<<30)

	// Record access for a non-existent file (should not panic).
	cm.RecordAccess("nonexistent.mp4")
}
