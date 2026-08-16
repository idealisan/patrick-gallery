package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildTimeBucketAssetsStackIsNullNotEmptyArray is a regression test for
// BUG-004 (the "NaN" text rendered above every thumbnail on the timeline).
//
// Root cause: immich-go emitted an empty array `[]` for each asset's slot in
// the parallel `stack` array. The official web (`timeline-month.svelte.ts`)
// treats a truthy slot as a stacked asset and computes
// `assetCount: Number.parseInt(slot[1])`. An empty `[]` is truthy in JS, the
// slot has no element [1] (undefined), and the parse yields NaN — which the
// thumbnail renders verbatim as "NaN". The official server emits `null` for a
// non-stacked asset, which the web treats as "not stacked" (no badge, no NaN).
//
// This test asserts the bucket response serializes each non-stacked slot as
// `null`, never as `[]`.
func TestBuildTimeBucketAssetsStackIsNullNotEmptyArray(t *testing.T) {
	// No ExifID on the asset, so buildTimeBucketAssets skips the store query
	// and can run without a live DB.
	assets := []Asset{
		{ID: "a1", Type: "IMAGE"},
		{ID: "a2", Type: "IMAGE"},
	}
	resp := (&App{}).buildTimeBucketAssets(assets)

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)

	if strings.Contains(out, `"stack":[[]]`) || strings.Contains(out, `"stack":[]`) && !strings.Contains(out, "null") {
		t.Fatalf("stack must not be an empty inner array; got: %s", out)
	}
	// Each slot must serialize as null (the contract for "not stacked").
	if !strings.Contains(out, `"stack":[null,null]`) {
		t.Fatalf("expected each stack slot to be null; got: %s", out)
	}
	// And never an empty inner array per slot.
	if strings.Contains(out, "[[]]") {
		t.Fatalf("stack slot must not be an empty array; got: %s", out)
	}
}

// TestBuildTimeBucketAssetsEmptyBucket keeps the parallel-array response
// contract intact (all arrays present, even when empty) while ensuring the
// stack array is `[]` (no slots) rather than containing empty-array slots.
func TestBuildTimeBucketAssetsEmptyBucket(t *testing.T) {
	resp := (&App{}).buildTimeBucketAssets(nil)
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"stack":[]`) {
		t.Fatalf("empty bucket must still emit a stack array; got: %s", out)
	}
}
