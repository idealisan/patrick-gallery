package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHLSVideoStreaming(t *testing.T) {
	app, r, token := newTestServer(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Resolve the seeded user id.
	meReq, _ := http.NewRequest("GET", srv.URL+"/api/users/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+token)
	meResp, err := http.DefaultClient.Do(meReq)
	if err != nil {
		t.Fatalf("users/me: %v", err)
	}
	meBody, _ := io.ReadAll(meResp.Body)
	meResp.Body.Close()
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(meBody, &me); err != nil || me.ID == "" {
		t.Fatalf("parse users/me: %v (%s)", err, string(meBody))
	}

	// Create a fake video asset pointing at a small file.
	tmp := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(tmp, []byte("fake-mp4-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	aid := newUUID()
	if err := app.store.DB.Create(&Asset{
		ID:               aid,
		OwnerID:          me.ID,
		Type:             "VIDEO",
		OriginalPath:     tmp,
		OriginalFileName: "clip.mp4",
		Duration:         "12.5",
	}).Error; err != nil {
		t.Fatal(err)
	}

	get := func(path string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(b)
	}

	// master playlist
	mResp, mBody := get("/api/assets/" + aid + "/video/stream/main.m3u8")
	if mResp.StatusCode != 200 {
		t.Fatalf("master status %d", mResp.StatusCode)
	}
	if !strings.Contains(mResp.Header.Get("Content-Type"), "vnd.apple.mpegurl") {
		t.Fatalf("master content-type %q", mResp.Header.Get("Content-Type"))
	}
	if !strings.Contains(mBody, "#EXTM3U") || !strings.Contains(mBody, "/playlist.m3u8") {
		t.Fatalf("master body unexpected: %q", mBody)
	}

	// variant playlist
	pResp, pBody := get("/api/assets/" + aid + "/video/stream/" + aid + "/0/playlist.m3u8")
	if pResp.StatusCode != 200 {
		t.Fatalf("playlist status %d", pResp.StatusCode)
	}
	if !strings.Contains(pBody, "#EXTINF") || !strings.Contains(pBody, "seg-0.mp4") || !strings.Contains(pBody, "#EXT-X-ENDLIST") {
		t.Fatalf("playlist body unexpected: %q", pBody)
	}

	// segment (transcode falls back to original in tests; still video/mp4)
	sResp, sBody := get("/api/assets/" + aid + "/video/stream/" + aid + "/0/seg-0.mp4")
	if sResp.StatusCode != 200 {
		t.Fatalf("segment status %d", sResp.StatusCode)
	}
	if !strings.Contains(sResp.Header.Get("Content-Type"), "video/mp4") {
		t.Fatalf("segment content-type %q", sResp.Header.Get("Content-Type"))
	}
	if len(sBody) == 0 {
		t.Fatal("segment body empty")
	}

	// direct playback
	pbResp, _ := get("/api/assets/" + aid + "/video/playback")
	if pbResp.StatusCode != 200 {
		t.Fatalf("playback status %d", pbResp.StatusCode)
	}

	// delete session -> 204
	delReq, _ := http.NewRequest("DELETE", srv.URL+"/api/assets/"+aid+"/video/stream/"+aid, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status %d", delResp.StatusCode)
	}
}
