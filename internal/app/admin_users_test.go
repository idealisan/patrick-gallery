package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// seededAdminID loads the seeded admin user id for the test store.
func seededAdminID(t *testing.T, app *App) string {
	t.Helper()
	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no admin seeded: %v", err)
	}
	return admin.ID
}

func TestAdminRequiresAdmin(t *testing.T) {
	app, r, adminToken := newTestServer(t)
	// Create a plain (non-admin) user directly and mint a token for it.
	nu := User{ID: newUUID(), Email: "user@example.com", Name: "Plain", Password: "x", IsAdmin: false, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
	if err := app.store.DB.Create(&nu).Error; err != nil {
		t.Fatal(err)
	}
	userToken, err := app.issueToken(nu.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = adminToken

	w := do(r, http.MethodGet, "/api/admin/users", userToken, nil, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestAdminUserLifecycle(t *testing.T) {
	_, r, token := newTestServer(t)

	// list
	w := do(r, http.MethodGet, "/api/admin/users", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list users: %d %s", w.Code, w.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least the seeded admin in the list")
	}
	if _, ok := list[0]["status"]; !ok {
		t.Fatal("admin user DTO missing status field")
	}

	// create
	body, _ := json.Marshal(map[string]any{
		"email":    "newuser@example.com",
		"name":     "New User",
		"password": "secret123",
		"isAdmin":  false,
	})
	w = do(r, http.MethodPost, "/api/admin/users", token, body, "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("created user missing id")
	}
	if created["status"] != "active" {
		t.Fatalf("expected status active, got %v", created["status"])
	}
	// password actually works
	login := do(r, http.MethodPost, "/api/auth/login", "", mustJSON(t, map[string]any{"email": "newuser@example.com", "password": "secret123"}), "application/json")
	if login.Code != http.StatusCreated {
		t.Fatalf("login as new user: %d %s", login.Code, login.Body.String())
	}

	// get
	w = do(r, http.MethodGet, "/api/admin/users/"+id, token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get user: %d", w.Code)
	}

	// update
	body, _ = json.Marshal(map[string]any{"name": "Renamed"})
	w = do(r, http.MethodPut, "/api/admin/users/"+id, token, body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("update user: %d %s", w.Code, w.Body.String())
	}
	var upd map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &upd)
	if upd["name"] != "Renamed" {
		t.Fatalf("name not updated: %v", upd["name"])
	}

	// statistics
	w = do(r, http.MethodGet, "/api/admin/users/"+id+"/statistics", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("statistics: %d", w.Code)
	}
	var stats map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &stats)
	for _, k := range []string{"images", "videos", "total"} {
		if _, ok := stats[k]; !ok {
			t.Fatalf("statistics missing %s", k)
		}
	}

	// sessions
	w = do(r, http.MethodGet, "/api/admin/users/"+id+"/sessions", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("sessions: %d", w.Code)
	}
	var sessions []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &sessions)
	// login above recorded a session
	if len(sessions) == 0 {
		t.Fatal("expected at least one session for the new user")
	}
	if _, ok := sessions[0]["current"]; !ok {
		t.Fatal("session missing current flag")
	}

	// calendar heatmap
	w = do(r, http.MethodGet, "/api/admin/users/"+id+"/calendar-heatmap", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("calendar heatmap: %d", w.Code)
	}
	var heat map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &heat)
	if _, ok := heat["series"]; !ok {
		t.Fatalf("heatmap missing series: %v", heat)
	}
	if _, ok := heat["totalCount"]; !ok {
		t.Fatalf("heatmap missing totalCount: %v", heat)
	}

	// preferences get
	w = do(r, http.MethodGet, "/api/admin/users/"+id+"/preferences", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("preferences get: %d", w.Code)
	}
	var prefs map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &prefs)
	if _, ok := prefs["albums"]; !ok {
		t.Fatal("preferences missing albums section")
	}

	// preferences put (partial merge)
	body, _ = json.Marshal(map[string]any{"albums": map[string]any{"defaultAssetOrder": "asc"}})
	w = do(r, http.MethodPut, "/api/admin/users/"+id+"/preferences", token, body, "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("preferences put: %d %s", w.Code, w.Body.String())
	}
	var prefs2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &prefs2)
	albums, _ := prefs2["albums"].(map[string]any)
	if albums["defaultAssetOrder"] != "asc" {
		t.Fatalf("preferences albums.defaultAssetOrder not merged: %v", albums)
	}
	// other sections preserved
	if _, ok := prefs2["download"]; !ok {
		t.Fatal("preferences put dropped other sections")
	}

	// delete (soft)
	w = do(r, http.MethodDelete, "/api/admin/users/"+id, token, mustJSON(t, map[string]any{"force": false}), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("delete user: %d %s", w.Code, w.Body.String())
	}
	var del map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &del)
	if del["status"] != "deleted" {
		t.Fatalf("expected status deleted after delete, got %v", del["status"])
	}

	// still visible (unscoped) with deleted status
	w = do(r, http.MethodGet, "/api/admin/users/"+id, token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get deleted user: %d", w.Code)
	}

	// restore
	w = do(r, http.MethodPost, "/api/admin/users/"+id+"/restore", token, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("restore user: %d %s", w.Code, w.Body.String())
	}
	var rest map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rest)
	if rest["status"] != "active" {
		t.Fatalf("expected status active after restore, got %v", rest["status"])
	}
}

func TestAdminCannotDeleteOnlyAdmin(t *testing.T) {
	app, r, token := newTestServer(t)
	id := seededAdminID(t, app)
	w := do(r, http.MethodDelete, "/api/admin/users/"+id, token, mustJSON(t, map[string]any{}), "application/json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when deleting the only admin, got %d", w.Code)
	}
}

func nowUTC() time.Time { return time.Now().UTC() }
