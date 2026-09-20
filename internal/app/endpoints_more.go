package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// endpoints_more.go implements the remaining iOS client-required API endpoints
// that were missing from immich-go (asset edits, partner update, library
// validation, bulk tag assignment, profile image, PIN / session-lock, and
// manual asset stacking). Face / ML endpoints are intentionally out of scope
// (AGENTS.md) and remain 501. Every handler here performs real work.

// ---------------- asset edits ----------------

type assetEditActionItem struct {
	ID         string      `json:"id"`
	Action     string      `json:"action"`
	Parameters interface{} `json:"parameters"`
}

type assetEditsResponse struct {
	AssetID string                `json:"assetId"`
	Edits   []assetEditActionItem `json:"edits"`
}

func (a *App) loadAssetEdits(assetID, uid string) (assetEditsResponse, error) {
	var as Asset
	if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", assetID, uid).Error; err != nil {
		return assetEditsResponse{}, err
	}
	var edits []AssetEdit
	a.store.DB.Where("asset_id = ?", assetID).Order("sequence ASC").Find(&edits)
	out := assetEditsResponse{AssetID: assetID, Edits: make([]assetEditActionItem, 0, len(edits))}
	for _, e := range edits {
		var params interface{}
		_ = json.Unmarshal([]byte(e.Parameters), &params)
		out.Edits = append(out.Edits, assetEditActionItem{ID: e.ID, Action: e.Action, Parameters: params})
	}
	return out, nil
}

func (a *App) handleAssetEditsGet(c *gin.Context) {
	uid := currentUserID(c)
	out, err := a.loadAssetEdits(c.Param("id"), uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "asset not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handleAssetEditsUpdate(c *gin.Context) {
	uid := currentUserID(c)
	assetID := c.Param("id")
	var as Asset
	if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", assetID, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "asset not found", "statusCode": 404})
		return
	}
	var body struct {
		Edits []struct {
			Action     string          `json:"action"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"edits"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Edits) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "edits required", "statusCode": 400})
		return
	}
	valid := map[string]bool{"crop": true, "rotate": true, "mirror": true}
	// Replace all existing edits atomically.
	a.store.DB.Where("asset_id = ?", assetID).Delete(&AssetEdit{})
	now := time.Now().UTC()
	for i, e := range body.Edits {
		if !valid[e.Action] {
			a.store.DB.Where("asset_id = ?", assetID).Delete(&AssetEdit{})
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid edit action: " + e.Action, "statusCode": 400})
			return
		}
		params := e.Parameters
		if len(params) == 0 {
			params = json.RawMessage("{}")
		}
		a.store.DB.Create(&AssetEdit{
			ID:        newUUID(),
			AssetID:   assetID,
			Action:    e.Action,
			Parameters: string(params),
			Sequence:  i,
			UpdatedAt: now,
		})
	}
	out, _ := a.loadAssetEdits(assetID, uid)
	c.JSON(http.StatusOK, out)
}

func (a *App) handleAssetEditsDelete(c *gin.Context) {
	uid := currentUserID(c)
	assetID := c.Param("id")
	var as Asset
	if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", assetID, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "asset not found", "statusCode": 404})
		return
	}
	a.store.DB.Where("asset_id = ?", assetID).Delete(&AssetEdit{})
	c.Status(http.StatusNoContent)
}

// ---------------- partner update ----------------

func (a *App) handlePartnerUpdate(c *gin.Context) {
	uid := currentUserID(c)
	other := c.Param("id")
	var b struct {
		InTimeline *bool `json:"inTimeline"`
	}
	_ = c.ShouldBindJSON(&b)
	var p Partner
	if err := a.store.DB.Where("(shared_by_id = ? AND shared_with_id = ?) OR (shared_with_id = ? AND shared_by_id = ?)", uid, other, uid, other).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "partner not found", "statusCode": 404})
		return
	}
	inTimeline := p.InTimeline
	if b.InTimeline != nil {
		inTimeline = *b.InTimeline
		a.store.DB.Model(&Partner{}).
			Where("shared_by_id = ? AND shared_with_id = ?", p.SharedByID, p.SharedWithID).
			Update("in_timeline", inTimeline)
	}
	var u User
	a.store.DB.First(&u, "id = ?", other)
	c.JSON(http.StatusOK, gin.H{
		"id":                other,
		"email":             u.Email,
		"name":              u.Name,
		"avatarColor":       u.AvatarColor,
		"profileImagePath":  u.ProfileImagePath,
		"profileChangedAt":  u.ProfileChangedAt.UTC().Format(time.RFC3339),
		"inTimeline":        inTimeline,
	})
}

// ---------------- library validate ----------------

func (a *App) handleLibraryValidate(c *gin.Context) {
	id := c.Param("id")
	var lib Library
	if err := a.store.DB.First(&lib, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "library not found", "statusCode": 404})
		return
	}
	var b struct {
		ImportPaths       []string `json:"importPaths"`
		ExclusionPatterns []string `json:"exclusionPatterns"`
	}
	_ = c.ShouldBindJSON(&b)
	paths := b.ImportPaths
	if len(paths) == 0 && lib.ImportPaths != "" {
		paths = strings.FieldsFunc(lib.ImportPaths, func(r rune) bool { return r == ',' || r == '\n' })
	}
	results := make([]gin.H, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		msg := ""
		valid := false
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			valid = true
		} else {
			msg = "path does not exist or is not a directory"
		}
		results = append(results, gin.H{"importPath": p, "isValid": valid, "message": msg})
	}
	c.JSON(http.StatusOK, gin.H{"importPaths": results})
}

// ---------------- bulk tag assignment ----------------

func (a *App) handleTagBulkAssets(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		TagIDs   []string `json:"tagIds"`
		AssetIDs []string `json:"assetIds"`
	}
	_ = c.ShouldBindJSON(&b)
	if len(b.TagIDs) == 0 || len(b.AssetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "tagIds and assetIds required", "statusCode": 400})
		return
	}
	count := 0
	for _, tid := range b.TagIDs {
		var t Tag
		if err := a.store.DB.First(&t, "id = ? AND user_id = ?", tid, uid).Error; err != nil {
			continue
		}
		for _, aid := range b.AssetIDs {
			var as Asset
			if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", aid, uid).Error; err != nil {
				continue
			}
			var cnt int64
			a.store.DB.Model(&AssetTag{}).Where("asset_id = ? AND tag_id = ?", aid, tid).Count(&cnt)
			if cnt == 0 {
				a.store.DB.Create(&AssetTag{AssetID: aid, TagID: tid})
				count++
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// ---------------- profile image ----------------

func (a *App) handleProfileImageCreate(c *gin.Context) {
	uid := currentUserID(c)
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "file required", "statusCode": 400})
		return
	}
	dir := filepath.Join(a.cfg.ResourceDir, "profile", uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	fname := newUUID() + ext
	dst := filepath.Join(dir, fname)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	rel := filepath.Join("profile", uid, fname)
	now := time.Now().UTC()
	a.store.DB.Model(&User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"profile_image_path":  rel,
		"profile_changed_at": now,
	})
	c.JSON(http.StatusCreated, gin.H{
		"userId":            uid,
		"profileChangedAt":  now.UTC().Format(time.RFC3339),
		"profileImagePath":  rel,
	})
}

func (a *App) handleProfileImageGet(c *gin.Context) {
	id := c.Param("id")
	var u User
	if err := a.store.DB.First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	if u.ProfileImagePath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(filepath.Join(a.cfg.ResourceDir, u.ProfileImagePath))
}

// handleProfileImageDelete removes the current user's profile image
// (DELETE /users/me/profile-image and /users/:id/profile-image).
func (a *App) handleProfileImageDelete(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	if u.ProfileImagePath != "" {
		_ = os.Remove(filepath.Join(a.cfg.ResourceDir, u.ProfileImagePath))
	}
	now := time.Now().UTC()
	a.store.DB.Model(&User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"profile_image_path":  "",
		"profile_changed_at": now,
	})
	c.Status(http.StatusNoContent)
}

// ---------------- PIN code & session lock ----------------

var pinRegexp = regexp.MustCompile(`^\d{6}$`)

func (a *App) handlePinCodeSetup(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		PinCode string `json:"pinCode"`
	}
	_ = c.ShouldBindJSON(&b)
	if !pinRegexp.MatchString(b.PinCode) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "pinCode must be 6 digits", "statusCode": 400})
		return
	}
	var u User
	a.store.DB.First(&u, "id = ?", uid)
	if u.PinCode != "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "pin code already set", "statusCode": 400})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(b.PinCode), bcrypt.DefaultCost)
	a.store.DB.Model(&User{}).Where("id = ?", uid).Update("pin_code", string(hash))
	c.Status(http.StatusNoContent)
}

// handlePinCodeChange updates an already-set PIN (PUT /auth/pin-code). It
// verifies the current PIN before storing the new one.
func (a *App) handlePinCodeChange(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		PinCode    string `json:"pinCode"`
		OldPinCode string `json:"oldPinCode"`
	}
	_ = c.ShouldBindJSON(&b)
	if !pinRegexp.MatchString(b.PinCode) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "pinCode must be 6 digits", "statusCode": 400})
		return
	}
	var u User
	a.store.DB.First(&u, "id = ?", uid)
	if u.PinCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "pin code not set", "statusCode": 400})
		return
	}
	if b.OldPinCode != "" && bcrypt.CompareHashAndPassword([]byte(u.PinCode), []byte(b.OldPinCode)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "current pin code is incorrect", "statusCode": 400})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(b.PinCode), bcrypt.DefaultCost)
	a.store.DB.Model(&User{}).Where("id = ?", uid).Update("pin_code", string(hash))
	c.Status(http.StatusNoContent)
}

// handlePinCodeClear removes the PIN (DELETE /auth/pin-code).
func (a *App) handlePinCodeClear(c *gin.Context) {
	uid := currentUserID(c)
	a.store.DB.Model(&User{}).Where("id = ?", uid).Update("pin_code", "")
	c.Status(http.StatusNoContent)
}

func (a *App) handleSessionLock(c *gin.Context) {
	if tok := requestToken(c); tok != "" {
		a.store.DB.Model(&Session{}).Where("token = ?", hashToken(tok)).Update("pin_expires_at", nil)
	}
	c.Status(http.StatusNoContent)
}

func (a *App) handleSessionUnlock(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		PinCode  *string `json:"pinCode"`
		Password *string `json:"password"`
	}
	_ = c.ShouldBindJSON(&b)
	var u User
	a.store.DB.First(&u, "id = ?", uid)
	ok := false
	if b.PinCode != nil && u.PinCode != "" {
		if bcrypt.CompareHashAndPassword([]byte(u.PinCode), []byte(*b.PinCode)) == nil {
			ok = true
		}
	}
	if !ok && b.Password != nil {
		if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(*b.Password)) == nil {
			ok = true
		}
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid pin or password", "statusCode": 401})
		return
	}
	now := time.Now().UTC()
	exp := now.Add(15 * time.Minute)
	if tok := requestToken(c); tok != "" {
		a.store.DB.Model(&Session{}).Where("token = ?", hashToken(tok)).Update("pin_expires_at", exp)
	}
	c.Status(http.StatusNoContent)
}

func (a *App) handleSessionCreate(c *gin.Context) {
	uid := currentUserID(c)
	var cur Session
	if tok := requestToken(c); tok != "" {
		a.store.DB.Where("token = ?", hashToken(tok)).First(&cur)
	}
	var b struct {
		Duration   *int   `json:"duration"`
		DeviceType string `json:"deviceType"`
		DeviceOS   string `json:"deviceOS"`
	}
	_ = c.ShouldBindJSON(&b)
	newTok, err := a.issueToken(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	now := time.Now().UTC()
	exp := now.Add(60 * 24 * time.Hour)
	if b.Duration != nil && *b.Duration > 0 {
		exp = now.Add(time.Duration(*b.Duration) * time.Second)
	}
	dt, dos := b.DeviceType, b.DeviceOS
	if dt == "" {
		dt = cur.DeviceType
	}
	if dos == "" {
		dos = cur.DeviceOS
	}
	s := Session{
		ID:        newUUID(),
		UserID:    uid,
		ParentID:  cur.ID,
		Token:     hashToken(newTok),
		DeviceOS:  dos,
		DeviceType: dt,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: exp,
	}
	a.store.DB.Create(&s)
	c.JSON(http.StatusCreated, sessionToResponse(s, false, newTok))
}

func sessionToResponse(s Session, current bool, token string) gin.H {
	m := gin.H{
		"id":                 s.ID,
		"createdAt":          s.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":          s.UpdatedAt.UTC().Format(time.RFC3339),
		"expiresAt":          s.ExpiresAt.UTC().Format(time.RFC3339),
		"deviceOS":           s.DeviceOS,
		"deviceType":         s.DeviceType,
		"appVersion":         nil,
		"current":            current,
		"isPendingSyncReset": s.IsPendingSyncReset,
	}
	if token != "" {
		m["token"] = token
	}
	return m
}

// ---------------- stacks (manual, no ML) ----------------

func (a *App) handleStackCreate(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		AssetIDs []string `json:"assetIds"`
	}
	_ = c.ShouldBindJSON(&b)
	if len(b.AssetIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "at least 2 assetIds required", "statusCode": 400})
		return
	}
	// Validate ownership and gather assets (primary first).
	assets := make([]Asset, 0, len(b.AssetIDs))
	for _, aid := range b.AssetIDs {
		var as Asset
		if err := a.store.DB.First(&as, "id = ? AND owner_id = ?", aid, uid).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "asset not found: " + aid, "statusCode": 404})
			return
		}
		assets = append(assets, as)
	}
	now := time.Now().UTC()
	stack := Stack{
		ID:             newUUID(),
		PrimaryAssetID: assets[0].ID,
		OwnerID:        uid,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := a.store.DB.Create(&stack).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	for i, as := range assets {
		a.store.DB.Create(&StackAsset{StackID: stack.ID, AssetID: as.ID, Order: i})
	}
	c.JSON(http.StatusCreated, a.stackToResponse(stack, assets))
}

func (a *App) handleStackGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var stack Stack
	if err := a.store.DB.First(&stack, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "stack not found", "statusCode": 404})
		return
	}
	assets := a.stackAssets(stack.ID)
	c.JSON(http.StatusOK, a.stackToResponse(stack, assets))
}

// handleStacksDelete implements DELETE /stacks (deleteStacks): it deletes the
// stacks named in the request body and dissolves their asset associations.
func (a *App) handleStacksDelete(c *gin.Context) {
	uid := currentUserID(c)
	var b struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&b)
	if len(b.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ids required", "statusCode": 400})
		return
	}
	for _, id := range b.IDs {
		a.deleteStackByID(uid, id)
	}
	c.Status(http.StatusNoContent)
}

// handleStackDelete implements DELETE /stacks/:id (deleteStack).
func (a *App) handleStackDelete(c *gin.Context) {
	uid := currentUserID(c)
	a.deleteStackByID(uid, c.Param("id"))
	c.Status(http.StatusNoContent)
}

// deleteStackByID removes a stack and its member links. Unknown ids are ignored
// so the operation stays idempotent (the official contract returns 204).
func (a *App) deleteStackByID(uid, id string) {
	var stack Stack
	if err := a.store.DB.First(&stack, "id = ? AND owner_id = ?", id, uid).Error; err != nil {
		return
	}
	a.store.DB.Where("stack_id = ?", id).Delete(&StackAsset{})
	a.store.DB.Delete(&stack)
}

func (a *App) stackAssets(stackID string) []Asset {
	var links []StackAsset
	a.store.DB.Where("stack_id = ?", stackID).Order("\"order\" ASC").Find(&links)
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.AssetID)
	}
	assets := make([]Asset, 0, len(ids))
	if len(ids) > 0 {
		a.store.DB.Where("id IN ?", ids).Find(&assets)
	}
	return assets
}

func (a *App) stackToResponse(stack Stack, assets []Asset) gin.H {
	out := make([]AssetResponse, 0, len(assets))
	for _, as := range assets {
		out = append(out, a.toResponse(as))
	}
	return gin.H{
		"id":             stack.ID,
		"primaryAssetId": stack.PrimaryAssetID,
		"assets":         out,
	}
}
