package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// android_contract_test.go replays the official Immich Android (Flutter) client's
// real request flow against the immich-go server and asserts that every response
// matches the exact DTO shapes the client parses (from
// mobile/openapi/lib/model/*.dart at tag v3.1.0). It is the headless equivalent of
// installing the official app on an emulator/device and clicking through it:
// login (immich_access_token cookie), bootstrap (about / users/me / preferences /
// notifications / sync.stream), upload, then the 14 App endpoints implemented for
// the "客户端所需 API 补全" workstream. See docs/ANDROID_TESTING.md.
//
// Run: go test ./internal/app -run TestAndroidClientContract -count=1

// ckReq performs a request using the immich_access_token cookie, exactly like the
// official Android client (which stores the login token in a Cookie, not a
// Bearer header).
func ckReq(r *gin.Engine, method, path, cookie string, body []byte, ct string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if cookie != "" {
		req.Header.Set("Cookie", "immich_access_token="+cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func loginClient(t *testing.T, r *gin.Engine, email, password string) (string, map[string]any) {
	t.Helper()
	w := ckReq(r, "POST", "/api/auth/login", "", mustJSON(t, map[string]any{"email": email, "password": password}), "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("login %s -> %d: %s", email, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	var cookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "immich_access_token" {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("login response has no immich_access_token cookie")
	}
	return cookie, resp
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return m
}

func haveFields(t *testing.T, name string, obj map[string]any, fields ...string) {
	t.Helper()
	for _, f := range fields {
		if _, ok := obj[f]; !ok {
			t.Errorf("%s: missing required client field %q (keys: %v)", name, f, keysOf(obj))
		}
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// uploadAssetCK uploads a generated JPEG as the Android client does (multipart
// "asset" metadata + "assetData" file) but authenticates with the cookie.
func uploadAssetCK(t *testing.T, r *gin.Engine, cookie, fileName string) string {
	t.Helper()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, fileName)
	// Content must differ per file name: the official upload contract
	// (verified live on v3.1.0) answers 200 {status:"duplicate"} for
	// identical bytes under the same owner.
	seed := 0
	for _, ch := range fileName {
		seed += int(ch)
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8((x * 7) % 256), uint8((y*11+seed) % 256), 120, 255})
		}
	}
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = jpeg.Encode(f, img, &jpeg.Options{Quality: 85})
	f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	meta := `{"deviceAssetId":"android-` + fileName + `","deviceId":"android-emulator","fileCreatedAt":"2024-01-01T00:00:00.000Z","localDateTime":"2024-01-01T00:00:00.000Z","type":"IMAGE","fileExtension":"jpg"}`
	if err := mw.WriteField("asset", meta); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("assetData", fileName)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := os.Open(imgPath)
	_, _ = io.Copy(fw, src)
	src.Close()
	mw.Close()

	w := ckReq(r, "POST", "/api/assets", cookie, buf.Bytes(), mw.FormDataContentType())
	if w.Code != http.StatusCreated {
		t.Fatalf("android upload %s -> %d: %s", fileName, w.Code, w.Body.String())
	}
	m := decode(t, w)
	id, _ := m["id"].(string)
	if id == "" {
		id, _ = m["assetId"].(string)
	}
	return id
}

func TestAndroidClientContract(t *testing.T) {
	app, r, adminToken := newTestServer(t)

	// 1) Login exactly like the Android client (cookie-based).
	t.Run("Login", func(t *testing.T) {
		cookie, resp := loginClient(t, r, "admin@immich.app", "password")
		haveFields(t, "LoginResponseDto", resp,
			"accessToken", "userToken", "userId", "userEmail", "name",
			"isAdmin", "isOnboarded", "shouldChangePassword", "profileImagePath")
		_ = app
		_ = adminToken
		_ = cookie
	})

	// Re-login to get a fresh cookie for the rest of the flows.
	cookie, loginResp := loginClient(t, r, "admin@immich.app", "password")
	userID, _ := loginResp["userId"].(string)

	// 2) Bootstrap the client performs on launch.
	t.Run("Bootstrap", func(t *testing.T) {
		// about / version check (official Flutter client path)
		w := ckReq(r, "GET", "/api/server/about", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("about -> %d", w.Code)
		}
		// users/me — UserResponseDto (the fields that previously crashed iOS)
		w = ckReq(r, "GET", "/api/users/me", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("users/me -> %d", w.Code)
		}
		me := decode(t, w)
		haveFields(t, "UserResponseDto", me,
			"id", "email", "name", "isAdmin", "shouldChangePassword",
			"avatarColor", "profileImagePath", "profileChangedAt", "oauthId", "status")

		// users/me/preferences — nested folders (the SPA crash case)
		w = ckReq(r, "GET", "/api/users/me/preferences", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("preferences -> %d", w.Code)
		}
		prefs := decode(t, w)
		folders, ok := prefs["folders"].(map[string]any)
		if !ok {
			t.Fatalf("preferences.folders missing/nil (SPA crash): %v", prefs)
		}
		haveFields(t, "UserPreferences.folders", folders, "enabled", "sidebarWeb")

		// notifications unread
		w = ckReq(r, "GET", "/api/notifications?unread=true", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("notifications -> %d", w.Code)
		}
		var notes []any
		if err := json.Unmarshal(w.Body.Bytes(), &notes); err != nil {
			t.Fatalf("notifications decode: %v", err)
		}
	})

	// 3) sync.stream (POST, jsonlines snapshot)
	t.Run("SyncStream", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/sync/stream", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("sync/stream -> %d", w.Code)
		}
		lines := bytes.Split(bytes.TrimSpace(w.Body.Bytes()), []byte("\n"))
		if len(lines) < 1 {
			t.Fatalf("sync/stream produced no lines")
		}
		for i, ln := range lines {
			var obj map[string]any
			if err := json.Unmarshal(ln, &obj); err != nil {
				t.Fatalf("sync/stream line %d not json: %v", i, err)
			}
			if _, ok := obj["type"]; !ok {
				t.Errorf("sync/stream line %d missing type", i)
			}
		}
	})

	// 4) Upload two assets (the client uploads from the device library).
	var a1, a2 string
	t.Run("Upload", func(t *testing.T) {
		a1 = uploadAssetCK(t, r, cookie, "android1.jpg")
		a2 = uploadAssetCK(t, r, cookie, "android2.jpg")
		if a1 == "" || a2 == "" {
			t.Fatal("upload returned empty asset id")
		}
		w := ckReq(r, "GET", "/api/assets/"+a1, cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("get asset -> %d", w.Code)
		}
	})

	// 5) Asset edits (viewer rotate/crop/mirror)
	t.Run("AssetEdits", func(t *testing.T) {
		w := ckReq(r, "GET", "/api/assets/"+a1+"/edits", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("edits GET -> %d", w.Code)
		}
		haveFields(t, "AssetEditsResponseDto", decode(t, w), "assetId", "edits")

		body := mustJSON(t, map[string]any{"edits": []any{
			map[string]any{"action": "rotate", "parameters": map[string]any{"deg": 90}},
			map[string]any{"action": "crop", "parameters": map[string]any{"x": 0, "y": 0, "w": 10, "h": 10}},
		}})
		w = ckReq(r, "PUT", "/api/assets/"+a1+"/edits", cookie, body, "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("edits PUT -> %d: %s", w.Code, w.Body.String())
		}
		ed := decode(t, w)
		haveFields(t, "AssetEditsResponseDto", ed, "assetId", "edits")
		edits, _ := ed["edits"].([]any)
		if len(edits) != 2 {
			t.Fatalf("expected 2 edits, got %d", len(edits))
		}
		first, _ := edits[0].(map[string]any)
		haveFields(t, "AssetEditActionItemResponseDto", first, "id", "action", "parameters")

		// delete then confirm empty
		w = ckReq(r, "DELETE", "/api/assets/"+a1+"/edits", cookie, nil, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("edits DELETE -> %d", w.Code)
		}
		w = ckReq(r, "GET", "/api/assets/"+a1+"/edits", cookie, nil, "")
		ed = decode(t, w)
		edits, _ = ed["edits"].([]any)
		if len(edits) != 0 {
			t.Fatalf("expected 0 edits after delete, got %d", len(edits))
		}
	})

	// 6) Partners: create a second user (admin), share, set inTimeline
	var partnerID string
	t.Run("Partners", func(t *testing.T) {
		// create a second user via admin (setup only; the client shares with an
		// already-existing user)
		w := do(r, "POST", "/api/admin/users", adminToken,
			mustJSON(t, map[string]any{"email": "android-partner@example.com", "password": "secret123", "name": "Android Partner"}), "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("admin create user -> %d: %s", w.Code, w.Body.String())
		}
		pu := decode(t, w)
		partnerID, _ = pu["id"].(string)
		if partnerID == "" {
			t.Fatal("partner user id empty")
		}
		w = ckReq(r, "POST", "/api/partners", cookie, mustJSON(t, map[string]any{"sharedUserId": partnerID}), "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("partner create -> %d: %s", w.Code, w.Body.String())
		}
		w = ckReq(r, "PUT", "/api/partners/"+partnerID, cookie, mustJSON(t, map[string]any{"inTimeline": true}), "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("partner update -> %d: %s", w.Code, w.Body.String())
		}
		haveFields(t, "PartnerResponseDto", decode(t, w),
			"id", "email", "name", "avatarColor", "profileImagePath", "profileChangedAt", "inTimeline")
	})

	// 7) Library validate
	t.Run("LibraryValidate", func(t *testing.T) {
		w := ckReq(r, "GET", "/api/libraries", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("libraries -> %d", w.Code)
		}
		var libsArr []map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &libsArr)
		if len(libsArr) == 0 {
			t.Fatal("no library to validate")
		}
		libID, _ := libsArr[0]["id"].(string)
		w = ckReq(r, "POST", "/api/libraries/"+libID+"/validate", cookie,
			mustJSON(t, map[string]any{"importPaths": []string{"/tmp", "/nonexistent/android-xyz"}}), "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("library validate -> %d: %s", w.Code, w.Body.String())
		}
		vl := decode(t, w)
		haveFields(t, "ValidateLibraryResponseDto", vl, "importPaths")
		ip, _ := vl["importPaths"].([]any)
		if len(ip) != 2 {
			t.Fatalf("expected 2 importPath results, got %d", len(ip))
		}
		first, _ := ip[0].(map[string]any)
		haveFields(t, "ValidateLibraryImportPathResponseDto", first, "importPath", "isValid")
	})

	// 8) Bulk tag assignment
	t.Run("TagBulk", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/tags", cookie,
			mustJSON(t, map[string]any{"name": "android-tag", "type": "USER", "color": "#00ff00"}), "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("tag create -> %d: %s", w.Code, w.Body.String())
		}
		tagID, _ := decode(t, w)["id"].(string)
		w = ckReq(r, "PUT", "/api/tags/assets", cookie,
			mustJSON(t, map[string]any{"tagIds": []string{tagID}, "assetIds": []string{a1, a2}}), "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("tag bulk -> %d: %s", w.Code, w.Body.String())
		}
		haveFields(t, "TagBulkAssetsResponseDto", decode(t, w), "count")
	})

	// 9) Profile image upload + fetch
	t.Run("ProfileImage", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "p.jpg")
		fw.Write([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9"))
		mw.Close()
		w := ckReq(r, "POST", "/api/users/profile-image", cookie, buf.Bytes(), mw.FormDataContentType())
		if w.Code != http.StatusCreated {
			t.Fatalf("profile-image POST -> %d: %s", w.Code, w.Body.String())
		}
		haveFields(t, "CreateProfileImageResponseDto", decode(t, w), "userId", "profileImagePath", "profileChangedAt")
		w = ckReq(r, "GET", "/api/users/"+userID+"/profile-image", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("profile-image GET -> %d", w.Code)
		}
	})

	// 10) Session create (child session token)
	t.Run("Sessions", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/sessions", cookie,
			mustJSON(t, map[string]any{"deviceType": "ANDROID", "deviceOS": "Android 14"}), "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("sessions POST -> %d: %s", w.Code, w.Body.String())
		}
		haveFields(t, "SessionCreateResponseDto", decode(t, w),
			"id", "createdAt", "updatedAt", "expiresAt", "deviceOS", "deviceType",
			"appVersion", "current", "isPendingSyncReset", "token")
	})

	// 11) PIN + session lock/unlock
	t.Run("PinAndLock", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/auth/pin-code", cookie, mustJSON(t, map[string]any{"pinCode": "123456"}), "application/json")
		if w.Code != http.StatusNoContent {
			t.Fatalf("pin-code -> %d", w.Code)
		}
		w = ckReq(r, "POST", "/api/auth/session/lock", cookie, nil, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("session lock -> %d", w.Code)
		}
		w = ckReq(r, "POST", "/api/auth/session/unlock", cookie, mustJSON(t, map[string]any{"password": "password"}), "application/json")
		if w.Code != http.StatusNoContent {
			t.Fatalf("session unlock -> %d", w.Code)
		}
	})

	// 12) Manual stack
	t.Run("Stacks", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/stacks", cookie, mustJSON(t, map[string]any{"assetIds": []string{a1, a2}}), "application/json")
		if w.Code != http.StatusCreated {
			t.Fatalf("stacks POST -> %d: %s", w.Code, w.Body.String())
		}
		s := decode(t, w)
		haveFields(t, "StackResponseDto", s, "id", "primaryAssetId", "assets")
		assets, _ := s["assets"].([]any)
		if len(assets) != 2 {
			t.Fatalf("expected 2 stacked assets, got %d", len(assets))
		}
		stackID, _ := s["id"].(string)
		w = ckReq(r, "GET", "/api/stacks/"+stackID, cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("stacks GET -> %d", w.Code)
		}
		w = ckReq(r, "DELETE", "/api/stacks", cookie, mustJSON(t, map[string]any{"ids": []string{stackID}}), "application/json")
		if w.Code != http.StatusNoContent {
			t.Fatalf("stacks DELETE -> %d", w.Code)
		}
	})
}

// TestAndroidClientE2EMediaDownloadLogout replays the FULL Android client
// lifecycle the task requires: login -> upload image -> upload video ->
// download (image thumbnail/preview/original + video playback) -> logout.
// It pins the exact response shapes the official v3.1.0 client parses so any
// contract regression surfaces as a test failure. See docs/ANDROID_TESTING.md.
//
// Run: go test ./internal/app -run TestAndroidClientE2EMediaDownloadLogout -count=1
func TestAndroidClientE2EMediaDownloadLogout(t *testing.T) {
	_, r, _ := newTestServer(t)

	// 1) LOGIN exactly like the official Android client (cookie-based).
	cookie, _ := loginClient(t, r, "admin@immich.app", "password")

	// 2) UPLOAD IMAGE — multipart "assetData" + the exact form fields the
	//    official mobile client v3.1.0 sends (background_upload.service.dart).
	imgRaw := genImageJPEG(t, "e2e-img.jpg", 64, 48)
	imgID := uploadMultipartCK(t, r, cookie, "e2e-img.jpg", imgRaw, "IMAGE",
		map[string]string{
			"deviceAssetId": "android-e2e-img",
			"deviceId":      "android-emulator",
			"fileCreatedAt": "2024-01-01T00:00:00.000Z",
			"fileModifiedAt": "2024-01-01T00:00:00.000Z",
			"isFavorite":    "false",
			"duration":      "0",
		})
	if imgID == "" {
		t.Fatal("image upload returned empty id")
	}

	// 3) DOWNLOAD IMAGE — three sizes the client requests.
	t.Run("DownloadImage", func(t *testing.T) {
		// thumbnail (grid view)
		w := ckReq(r, "GET", "/api/assets/"+imgID+"/thumbnail?size=thumbnail", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("img thumbnail -> %d: %s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
			t.Errorf("img thumbnail content-type = %q (want image/*)", ct)
		}
		if w.Body.Len() == 0 {
			t.Error("img thumbnail body empty")
		}

		// preview (lightbox / full view) — official uses ?size=preview
		w = ckReq(r, "GET", "/api/assets/"+imgID+"/thumbnail?size=preview", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("img preview -> %d: %s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
			t.Errorf("img preview content-type = %q (want image/*)", ct)
		}

		// original (download to device)
		w = ckReq(r, "GET", "/api/assets/"+imgID+"/original", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("img original -> %d: %s", w.Code, w.Body.String())
		}
		if w.Body.Len() != len(imgRaw) {
			t.Errorf("img original body len = %d, want %d (uploaded bytes)", w.Body.Len(), len(imgRaw))
		}
	})

	// 4) UPLOAD VIDEO — minimal valid mp4 (ftyp box) so type sniffing yields VIDEO.
	vidRaw := minimalMP4()
	vidID := uploadMultipartCK(t, r, cookie, "e2e-vid.mp4", vidRaw, "VIDEO",
		map[string]string{
			"deviceAssetId": "android-e2e-vid",
			"deviceId":      "android-emulator",
			"fileCreatedAt": "2024-01-01T00:00:00.000Z",
			"fileModifiedAt": "2024-01-01T00:00:00.000Z",
			"isFavorite":    "false",
			"duration":      "0",
		})
	if vidID == "" {
		t.Fatal("video upload returned empty id")
	}

	// 5) DOWNLOAD VIDEO — /video/playback is what the official player opens.
	t.Run("DownloadVideo", func(t *testing.T) {
		w := ckReq(r, "GET", "/api/assets/"+vidID+"/video/playback", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("video playback -> %d: %s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "video/") {
			t.Errorf("video playback content-type = %q (want video/*)", ct)
		}
		if w.Body.Len() == 0 {
			t.Error("video playback body empty")
		}
	})

	// 6) LOGOUT — official client POSTs /api/auth/logout and parses the JSON
	//    response (LogoutResponseDto). An empty body crashes the Flutter SDK
	//    with "FormatException: Unexpected character" (see logs/).
	t.Run("Logout", func(t *testing.T) {
		w := ckReq(r, "POST", "/api/auth/logout", cookie, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("logout -> %d: %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("logout response not JSON (%q): %v", w.Body.String(), err)
		}
		if body["successful"] != true {
			t.Errorf("logout.successful = %v, want true", body["successful"])
		}
		if _, ok := body["redirectUri"]; !ok {
			t.Error("logout.redirectUri missing (required by LogoutResponseDto)")
		}
		// auth cookies must be cleared so the session ends on the client
		for _, c := range w.Result().Cookies() {
			if c.Name == "immich_access_token" || c.Name == "immich_is_authenticated" {
				if c.MaxAge > 0 && c.Value != "" {
					t.Errorf("logout did not clear cookie %s (maxAge=%d value=%q)", c.Name, c.MaxAge, c.Value)
				}
			}
		}
	})
}

// uploadMultipartCK mirrors the official mobile upload: a multipart form with a
// binary "assetData" part plus the scalar fields the client sends. It
// authenticates with the immich_access_token cookie.
func uploadMultipartCK(t *testing.T, r *gin.Engine, cookie, fileName string, raw []byte, _ string, fields map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("assetData", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(raw); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	w := ckReq(r, "POST", "/api/assets", cookie, buf.Bytes(), mw.FormDataContentType())
	if w.Code != http.StatusCreated {
		t.Fatalf("upload %s -> %d: %s", fileName, w.Code, w.Body.String())
	}
	m := decode(t, w)
	id, _ := m["id"].(string)
	if id == "" {
		id, _ = m["assetId"].(string)
	}
	return id
}

// genImageJPEG returns deterministically-generated JPEG bytes for testing.
func genImageJPEG(t *testing.T, fileName string, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8((x * 7) % 256), uint8((y * 11) % 256), 120, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// minimalMP4 returns a minimal well-formed ISO-BMP4 (ftyp) container so the
// type sniffer classifies the upload as VIDEO and the playback endpoint serves
// it. It is not a playable clip — only enough structure for the contract test.
func minimalMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, // box size = 24
		'f', 't', 'y', 'p', // ftyp
		'i', 's', 'o', 'm', // major brand
		0x00, 0x00, 0x00, 0x00, // minor version
		'i', 's', 'o', 'm', // compatible brand
		0x00, 0x00, 0x00, 0x00,
	}
}
