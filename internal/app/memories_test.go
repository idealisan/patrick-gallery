package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestMemoriesOnThisDay(t *testing.T) {
	app, r, token := newTestServer(t)
	uid := adminID(t, app)

	// An asset "created" on a fixed month/day should surface under /api/memories.
	memTime := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := app.store.DB.Create(&Asset{
		ID:            newUUID(),
		OwnerID:       uid,
		Type:          "IMAGE",
		OriginalPath:  "x",
		FileCreatedAt: memTime,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	// A trashed asset must NOT appear.
	if err := app.store.DB.Create(&Asset{
		ID:            newUUID(),
		OwnerID:       uid,
		Type:          "IMAGE",
		OriginalPath:  "y",
		FileCreatedAt: memTime,
		IsTrash:       true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	day := "06-15"
	w := do(r, "GET", "/api/memories?day="+day, token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("/api/memories -> %d: %s", w.Code, w.Body.String())
	}
	var groups []struct {
		Years  []int `json:"years"`
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one memory group for 06-15")
	}
	// Exactly one non-trashed asset should be returned (the trashed one is filtered).
	total := 0
	for _, g := range groups {
		total += len(g.Assets)
	}
	if total != 1 {
		t.Fatalf("expected 1 memory asset, got %d", total)
	}

	// year filter narrows to the right group.
	w2 := do(r, "GET", "/api/memories?day="+day+"&year=2021", token, nil, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("/api/memories(year) -> %d", w2.Code)
	}
	var groups2 []struct {
		Years  []int `json:"years"`
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &groups2)
	if len(groups2) != 1 || groups2[0].Years[0] != 2021 {
		t.Fatalf("year filter: expected single 2021 group, got %+v", groups2)
	}
}

func TestMemoriesEmpty(t *testing.T) {
	_, r, token := newTestServer(t)
	// A day with no assets should return an empty (200) array, not an error.
	w := do(r, "GET", "/api/memories?day=01-01", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("/api/memories -> %d: %s", w.Code, w.Body.String())
	}
	var groups []any
	if err := json.Unmarshal(w.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected empty memories, got %d groups", len(groups))
	}
}

func TestNotificationRegisterAndRemove(t *testing.T) {
	_, r, token := newTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"deviceToken": "test-token-abc",
		"platform":    "ios",
	})
	w := do(r, "POST", "/api/notifications", token, body, "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("register -> %d: %s", w.Code, w.Body.String())
	}
	// Re-registering the same token must not error (upsert).
	w2 := do(r, "POST", "/api/notifications", token, body, "application/json")
	if w2.Code != http.StatusCreated {
		t.Fatalf("re-register -> %d: %s", w2.Code, w2.Body.String())
	}
	// Missing token => 400.
	w3 := do(r, "POST", "/api/notifications", token, []byte("{}"), "application/json")
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("missing token -> %d (want 400)", w3.Code)
	}
	// Remove.
	w4 := do(r, "DELETE", "/api/notifications", token, nil, "")
	if w4.Code != http.StatusOK {
		t.Fatalf("remove -> %d: %s", w4.Code, w4.Body.String())
	}
}

func TestOAuthConfigDisabled(t *testing.T) {
	_, r, token := newTestServer(t)
	w := do(r, "GET", "/api/oauth/config", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("/api/oauth/config -> %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Enabled {
		t.Fatal("expected oauth enabled=false")
	}
}
