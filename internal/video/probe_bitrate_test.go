package video

import (
	"fmt"
	"testing"
)

func TestProbeBitrate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	p := New()
	if _, ok := p.(*placeholder); ok {
		t.Skip("placeholder backend — no FFmpeg available")
	}

	paths := []string{
		"/Users/songcheng/Developer/immich-go/resources/upload/298e3847-9fca-4634-af1b-3c3324af7e5c.mov",
		"/Users/songcheng/Developer/immich-go/resources/upload/842ee86a-abd7-48ad-a493-75c26540aab9.mp4",
		"/Users/songcheng/Developer/immich-go/resources/upload/c7e97443-d275-48c5-86a6-f26738af9712.mp4",
	}

	for _, path := range paths {
		meta, err := p.ProbeFile(path)
		if err != nil {
			fmt.Printf("ProbeFile(%s) error: %v\n", path, err)
			continue
		}
		fmt.Printf("ProbeFile(%s): W=%d H=%d Bitrate=%d Dur=%.2f Video=%s Audio=%s\n",
			path, meta.Width, meta.Height, meta.Bitrate, meta.DurationSec, meta.VideoCodec, meta.AudioCodec)

		thresholds := DefaultBitrateThresholds()
		need, max := thresholds.NeedsTranscode(meta.Width, meta.Bitrate)
		fmt.Printf("  -> NeedsTranscode=%v maxBitrate=%d\n", need, max)
	}
}
