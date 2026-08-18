package video

// transcode_decision.go — decides whether a video needs transcoding based on
// resolution and bitrate thresholds, and provides disk cache management.

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// BitrateThresholds defines the maximum streaming bitrate for each resolution
// tier. If the original video's bitrate exceeds the threshold, it must be
// transcoded even if the browser supports the codec natively.
type BitrateThresholds struct {
	// MaxBitrateBps maps width thresholds to max bitrate in bits/sec.
	// Keys are the maximum width for that tier.
	MaxBitrateBps map[int]int64
}

// DefaultBitrateThresholds returns the standard thresholds from the design doc.
func DefaultBitrateThresholds() BitrateThresholds {
	return BitrateThresholds{
		MaxBitrateBps: map[int]int64{
			1920: 3_000_000,  // ≤1080p: 3 Mbps
			2560: 5_000_000,  // 2K: 5 Mbps
			3840: 8_000_000,  // 4K: 8 Mbps
		},
	}
}

// NeedsTranscode reports whether the video should be transcoded based on its
// resolution and bitrate. Returns (needsTranscode, maxBitrateBps).
// If the original bitrate is within the threshold for its resolution, no
// transcoding is needed.
func (t BitrateThresholds) NeedsTranscode(width int, bitrateBps int64) (bool, int64) {
	maxBitrate := t.maxBitrateForWidth(width)
	if bitrateBps <= 0 {
		// Unknown bitrate -> transcode to be safe.
		return true, maxBitrate
	}
	return bitrateBps > maxBitrate, maxBitrate
}

func (t BitrateThresholds) maxBitrateForWidth(width int) int64 {
	// Find the smallest tier that fits this width.
	type tier struct {
		width    int
		bitrate  int64
	}
	var tiers []tier
	for w, b := range t.MaxBitrateBps {
		tiers = append(tiers, tier{w, b})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].width < tiers[j].width })

	for _, t := range tiers {
		if width <= t.width {
			return t.bitrate
		}
	}
	// Beyond 4K: use the highest tier.
	if len(tiers) > 0 {
		return tiers[len(tiers)-1].bitrate
	}
	return 8_000_000
}

// CacheManager manages a disk cache of transcoded video files with LRU eviction.
type CacheManager struct {
	dir       string
	maxSize   int64 // max total size in bytes
	mu        sync.Mutex
	lastUsed  map[string]time.Time
	totalSize int64
}

// NewCacheManager creates a cache manager for the given directory.
func NewCacheManager(dir string, maxSizeBytes int64) *CacheManager {
	cm := &CacheManager{
		dir:      dir,
		maxSize:  maxSizeBytes,
		lastUsed: make(map[string]time.Time),
	}
	cm.scan()
	return cm
}

// scan walks the cache directory and computes total size.
func (cm *CacheManager) scan() {
	cm.totalSize = 0
	entries, err := os.ReadDir(cm.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cm.totalSize += info.Size()
		cm.lastUsed[e.Name()] = info.ModTime()
	}
}

// RecordAccess updates the last-used time for a cache entry.
func (cm *CacheManager) RecordAccess(filename string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.lastUsed[filename] = time.Now()
}

// Evict removes the least recently used files until total size <= maxSize.
// Returns the list of evicted filenames.
func (cm *CacheManager) Evict() []string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.totalSize <= cm.maxSize {
		return nil
	}

	type entry struct {
		name string
		time time.Time
	}
	var entries []entry
	for name, t := range cm.lastUsed {
		entries = append(entries, entry{name, t})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].time.Before(entries[j].time)
	})

	var evicted []string
	for _, e := range entries {
		if cm.totalSize <= cm.maxSize {
			break
		}
		path := filepath.Join(cm.dir, e.name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			continue
		}
		cm.totalSize -= info.Size()
		delete(cm.lastUsed, e.name)
		evicted = append(evicted, e.name)
	}
	return evicted
}

// Add records a new file in the cache.
func (cm *CacheManager) Add(filename string, size int64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.lastUsed[filename] = time.Now()
	cm.totalSize += size
}

// TotalSize returns the current total cache size in bytes.
func (cm *CacheManager) TotalSize() int64 {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.totalSize
}
