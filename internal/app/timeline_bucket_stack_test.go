package app

import (
	"encoding/json"
	"testing"
)

// The official v3.1.0 bucket response (verified live against a running
// immich/immich-server:v3.1.0) is a parallel-array object with EXACTLY these
// keys: city, country, createdAt, duration, fileCreatedAt, id, isFavorite,
// isImage, isTrashed, livePhotoVideoId, localOffsetHours, ownerId,
// projectionType, ratio, status, thumbhash, visibility.
//
// In particular it has NO stack, latitude or longitude slots. That also
// retires the historical BUG-004 ("NaN" badge on the timeline): the web never
// sees a stack slot in this response, so a malformed one can no longer be
// rendered as "NaN".
//
// Duration slots are `null` for assets without a duration (still photos);
// status is "active" (or "trashed").
func TestBuildTimeBucketAssetsMatchesOfficialShape(t *testing.T) {
	assets := []Asset{
		{ID: "a1", Type: "IMAGE"},
		{ID: "a2", Type: "IMAGE"},
	}
	resp := (&App{}).buildTimeBucketAssets(assets)

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, gone := range []string{"stack", "latitude", "longitude"} {
		if _, ok := m[gone]; ok {
			t.Fatalf("official v3.1.0 bucket response has no %q key; got: %s", gone, b)
		}
	}
	for _, want := range []string{"status", "city", "country", "id", "ownerId", "visibility"} {
		if _, ok := m[want]; !ok {
			t.Fatalf("official v3.1.0 bucket response must carry %q; got: %s", want, b)
		}
	}

	status, ok := m["status"].([]any)
	if !ok || len(status) != 2 || status[0] != "active" || status[1] != "active" {
		t.Fatalf("status slots must be [active active]; got: %v", m["status"])
	}
	dur, ok := m["duration"].([]any)
	if !ok || len(dur) != 2 || dur[0] != nil || dur[1] != nil {
		t.Fatalf("duration slots for still photos must be null; got: %v", m["duration"])
	}
}

// An empty bucket must still serialize every parallel array (the schema
// requires them), with no leftover stack/latitude/longitude keys.
func TestBuildTimeBucketAssetsEmptyBucket(t *testing.T) {
	resp := (&App{}).buildTimeBucketAssets(nil)
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m) == 0 {
		t.Fatalf("empty bucket must still emit the parallel arrays; got: %s", b)
	}
	for k, v := range m {
		arr, ok := v.([]any)
		if !ok {
			t.Fatalf("field %s must be an array; got %T", k, v)
		}
		if len(arr) != 0 {
			t.Fatalf("empty bucket: %s must be empty; got %d slots", k, len(arr))
		}
	}
	if _, ok := m["stack"]; ok {
		t.Fatalf("official v3.1.0 bucket response has no stack key; got: %s", b)
	}
}
