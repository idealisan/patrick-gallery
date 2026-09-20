package app

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// requireAdmin aborts with 401/403 when the caller is not an administrator.
func (a *App) requireAdmin(c *gin.Context) (*User, bool) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "unauthorized", "statusCode": 401})
		return nil, false
	}
	if !u.IsAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "admin privileges required", "statusCode": 403})
		return nil, false
	}
	return &u, true
}

// ---- DTOs (mirror Immich v3.1.0 UserAdminResponseDto etc.) ----

type adminUserResponse struct {
	ID                   string     `json:"id"`
	Email                string     `json:"email"`
	Name                 string     `json:"name"`
	IsAdmin              bool       `json:"isAdmin"`
	AvatarColor          string     `json:"avatarColor"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	DeletedAt            *time.Time `json:"deletedAt"`
	ProfileChangedAt     time.Time  `json:"profileChangedAt"`
	ProfileImagePath     string     `json:"profileImagePath"`
	QuotaSizeInBytes     *int64     `json:"quotaSizeInBytes"`
	QuotaUsageInBytes    *int64     `json:"quotaUsageInBytes"`
	ShouldChangePassword bool       `json:"shouldChangePassword"`
	Status               string     `json:"status"`
	StorageLabel         *string    `json:"storageLabel"`
	OauthId              string     `json:"oauthId"`
	License              any        `json:"license"` // null: no license system
}

type userAdminCreateDto struct {
	Email                string `json:"email"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	IsAdmin              bool   `json:"isAdmin"`
	AvatarColor          string `json:"avatarColor"`
	PinCode              string `json:"pinCode"`
	QuotaSizeInBytes     *int64 `json:"quotaSizeInBytes"`
	ShouldChangePassword bool   `json:"shouldChangePassword"`
	StorageLabel         string `json:"storageLabel"`
}

type userAdminUpdateDto struct {
	Email                string `json:"email"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	IsAdmin              *bool  `json:"isAdmin"`
	AvatarColor          string `json:"avatarColor"`
	PinCode              string `json:"pinCode"`
	QuotaSizeInBytes     *int64 `json:"quotaSizeInBytes"`
	ShouldChangePassword *bool  `json:"shouldChangePassword"`
	StorageLabel         string `json:"storageLabel"`
}

type userAdminDeleteDto struct {
	Force bool `json:"force"`
}

// userStatus derives the Immich UserStatus enum from soft-delete state.
func userStatus(u User) string {
	if u.DeletedAt.Valid {
		return "deleted"
	}
	return "active"
}

// userQuotaUsage sums the bytes of a user's non-trashed assets.
func (a *App) userQuotaUsage(userID string) int64 {
	var usage int64
	a.store.DB.Raw(
		"SELECT COALESCE(SUM(size),0) FROM assets WHERE owner_id = ? AND is_trash = ?",
		userID, false,
	).Scan(&usage)
	return usage
}

// toAdminUser builds the contract DTO for a user row.
func (a *App) toAdminUser(u User) adminUserResponse {
	usage := a.userQuotaUsage(u.ID)
	var deletedAt *time.Time
	if u.DeletedAt.Valid {
		t := u.DeletedAt.Time
		deletedAt = &t
	}
	var storageLabel *string
	if u.StorageLabel != "" {
		s := u.StorageLabel
		storageLabel = &s
	}
	avatar := u.AvatarColor
	if avatar == "" {
		avatar = "primary"
	}
	pca := u.ProfileChangedAt
	if pca.IsZero() {
		pca = u.UpdatedAt
	}
	return adminUserResponse{
		ID:                   u.ID,
		Email:                u.Email,
		Name:                 u.Name,
		IsAdmin:              u.IsAdmin,
		AvatarColor:          avatar,
		CreatedAt:            u.CreatedAt,
		UpdatedAt:            u.UpdatedAt,
		DeletedAt:            deletedAt,
		ProfileChangedAt:     pca,
		ProfileImagePath:     u.ProfileImagePath,
		QuotaSizeInBytes:     u.QuotaSizeInBytes,
		QuotaUsageInBytes:    &usage,
		ShouldChangePassword: u.ShouldChangePassword,
		Status:               userStatus(u),
		StorageLabel:         storageLabel,
		OauthId:              u.OAuthId,
		License:              nil,
	}
}

// ---- handlers ----

func (a *App) handleAdminListUsers(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	// Unscoped so removed users (status=deleted) are visible to the admin.
	var users []User
	a.store.DB.Unscoped().Order("created_at ASC").Find(&users)
	out := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, a.toAdminUser(u))
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) handleAdminCreateUser(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var b userAdminCreateDto
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid body", "statusCode": 400})
		return
	}
	if b.Email == "" || b.Name == "" || b.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "email, name and password are required", "statusCode": 400})
		return
	}
	if b.PinCode != "" && len(b.PinCode) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "pinCode must be 6 digits", "statusCode": 400})
		return
	}
	var n int64
	a.store.DB.Model(&User{}).Where("email = ?", b.Email).Count(&n)
	if n > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "user already exists", "statusCode": 400})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(b.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	avatar := b.AvatarColor
	if avatar == "" {
		avatar = "primary"
	}
	now := time.Now().UTC()
	u := User{
		ID:                   newUUID(),
		Email:                b.Email,
		Name:                 b.Name,
		Password:             string(hash),
		Salt:                 newUUID(),
		IsAdmin:              b.IsAdmin,
		AvatarColor:          avatar,
		ShouldChangePassword: b.ShouldChangePassword,
		PinCode:              b.PinCode,
		QuotaSizeInBytes:     b.QuotaSizeInBytes,
		StorageLabel:         b.StorageLabel,
		CreatedAt:            now,
		UpdatedAt:            now,
		ProfileChangedAt:     now,
	}
	if err := a.store.DB.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	tok, _ := a.issueToken(u.ID)
	a.recordSession(u.ID, tok)
	c.JSON(http.StatusCreated, a.toAdminUser(u))
}

func (a *App) handleAdminGetUser(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var u User
	if err := a.store.DB.Unscoped().First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, a.toAdminUser(u))
}

func (a *App) handleAdminUpdateUser(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var u User
	if err := a.store.DB.Unscoped().First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	var b userAdminUpdateDto
	_ = c.ShouldBindJSON(&b)
	if b.Email != "" {
		u.Email = b.Email
	}
	if b.Name != "" {
		u.Name = b.Name
	}
	if b.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(b.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		u.Password = string(hash)
		u.ShouldChangePassword = false
	}
	// UserAdminUpdateDto fields are all optional — apply only what was sent.
	if b.IsAdmin != nil {
		u.IsAdmin = *b.IsAdmin
	}
	if b.AvatarColor != "" {
		u.AvatarColor = b.AvatarColor
	}
	u.PinCode = b.PinCode
	u.QuotaSizeInBytes = b.QuotaSizeInBytes
	u.StorageLabel = b.StorageLabel
	if b.ShouldChangePassword != nil {
		u.ShouldChangePassword = *b.ShouldChangePassword
	}
	u.UpdatedAt = time.Now().UTC()
	if err := a.store.DB.Save(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, a.toAdminUser(u))
}

func (a *App) handleAdminDeleteUser(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var u User
	if err := a.store.DB.Unscoped().First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	// Guard: never delete the only remaining admin.
	if u.IsAdmin {
		var adminCount int64
		a.store.DB.Model(&User{}).Where("is_admin = ?", true).Count(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "cannot delete the only admin", "statusCode": 400})
			return
		}
	}
	var b userAdminDeleteDto
	_ = c.ShouldBindJSON(&b)
	// Soft-delete the user (keeps the row so it can be restored).
	if err := a.store.DB.Delete(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	// Fan out a realtime user-delete event so connected official clients
	// (web/mobile) refresh / log the user out.
	a.emit("user.delete", map[string]any{"id": id})

	// When forced, also soft-delete the user's assets so they stop appearing.
	if b.Force {
		a.store.DB.Where("owner_id = ?", id).Delete(&Asset{})
	}
	// Reload to reflect deleted state.
	a.store.DB.Unscoped().First(&u, "id = ?", id)
	c.JSON(http.StatusOK, a.toAdminUser(u))
}

func (a *App) handleAdminRestoreUser(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var u User
	if err := a.store.DB.Unscoped().First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	if !u.DeletedAt.Valid {
		c.JSON(http.StatusOK, a.toAdminUser(u))
		return
	}
	if err := a.store.DB.Unscoped().Model(&u).Update("deleted_at", nil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	// Restore the user's assets too.
	a.store.DB.Unscoped().Model(&Asset{}).Where("owner_id = ?", id).Update("deleted_at", nil)
	a.store.DB.Unscoped().First(&u, "id = ?", id)
	c.JSON(http.StatusOK, a.toAdminUser(u))
}

// ---- sessions ----

type sessionResponse struct {
	ID                 string     `json:"id"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	DeviceOS           string     `json:"deviceOS"`
	DeviceType         string     `json:"deviceType"`
	AppVersion         *string    `json:"appVersion"`
	Current            bool       `json:"current"`
	IsPendingSyncReset bool       `json:"isPendingSyncReset"`
}

// recordSession persists a session row whenever a token is issued, so an admin
// can list a user's active sessions.
func (a *App) recordSession(userID, token string) Session {
	now := time.Now().UTC()
	s := Session{
		ID:        newUUID(),
		UserID:    userID,
		Token:     hashToken(token),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(60 * 24 * time.Hour),
	}
	_ = a.store.DB.Create(&s).Error
	return s
}

func (a *App) handleAdminUserSessions(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	var sessions []Session
	a.store.DB.Where("user_id = ?", id).Order("updated_at DESC").Find(&sessions)
	current := ""
	if len(sessions) > 0 {
		current = sessions[0].ID
	}
	out := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionResponse{
			ID:                 s.ID,
			CreatedAt:          s.CreatedAt,
			UpdatedAt:          s.UpdatedAt,
			ExpiresAt:          s.ExpiresAt,
			DeviceOS:           s.DeviceOS,
			DeviceType:         s.DeviceType,
			AppVersion:         nil,
			Current:            s.ID == current,
			IsPendingSyncReset: false,
		})
	}
	c.JSON(http.StatusOK, out)
}

// ---- statistics ----

func (a *App) handleAdminUserStatistics(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	if err := a.store.DB.First(&User{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	var photos, videos, total int64
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ?", id, false).Count(&total)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", id, false, "IMAGE").Count(&photos)
	a.store.DB.Model(&Asset{}).Where("owner_id = ? AND is_trash = ? AND type = ?", id, false, "VIDEO").Count(&videos)
	c.JSON(http.StatusOK, gin.H{"images": photos, "videos": videos, "total": total})
}

// ---- calendar heatmap ----

func (a *App) handleAdminUserCalendarHeatmap(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	if err := a.store.DB.First(&User{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	now := time.Now().UTC()
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -364).Format("2006-01-02")
	type dayCount struct {
		D string
		C int64
	}
	var rows []dayCount
	a.store.DB.Raw(
		"SELECT substr(local_date_time,1,10) AS d, COUNT(*) AS c FROM assets WHERE owner_id = ? AND local_date_time >= ? GROUP BY d",
		id, from,
	).Scan(&rows)
	counts := make(map[string]int64, len(rows))
	var total int64
	for _, r := range rows {
		counts[r.D] = r.C
		total += r.C
	}
	series := make([]gin.H, 0, 365)
	cur, _ := time.Parse("2006-01-02", from)
	for cur.Format("2006-01-02") <= to {
		d := cur.Format("2006-01-02")
		series = append(series, gin.H{"date": d, "count": counts[d]})
		cur = cur.AddDate(0, 0, 1)
	}
	c.JSON(http.StatusOK, gin.H{
		"from":       from,
		"to":         to,
		"series":     series,
		"totalCount": total,
	})
}

// ---- preferences ----

type prefAlbums struct {
	DefaultAssetOrder string `json:"defaultAssetOrder"`
}
type prefCast struct {
	GCastEnabled bool `json:"gCastEnabled"`
}
type prefDownload struct {
	ArchiveSize          int  `json:"archiveSize"`
	IncludeEmbeddedVideos bool `json:"includeEmbeddedVideos"`
}
type prefEmailNotif struct {
	AlbumInvite bool `json:"albumInvite"`
	AlbumUpdate bool `json:"albumUpdate"`
	Enabled     bool `json:"enabled"`
}
type prefFolders struct {
	Enabled    bool `json:"enabled"`
	SidebarWeb bool `json:"sidebarWeb"`
}
type prefMemories struct {
	Duration int  `json:"duration"`
	Enabled  bool `json:"enabled"`
}
type prefPeople struct {
	Enabled    bool `json:"enabled"`
	MinimumFaces int `json:"minimumFaces"`
	SidebarWeb  bool `json:"sidebarWeb"`
}
type prefPurchase struct {
	HideBuyButtonUntil string `json:"hideBuyButtonUntil"`
	ShowSupportBadge   bool   `json:"showSupportBadge"`
}
type prefRatings struct {
	Enabled bool `json:"enabled"`
}
type prefRecentlyAdded struct {
	SidebarWeb bool `json:"sidebarWeb"`
}
type prefSharedLinks struct {
	Enabled    bool `json:"enabled"`
	SidebarWeb bool `json:"sidebarWeb"`
}
type prefTags struct {
	Enabled    bool `json:"enabled"`
	SidebarWeb bool `json:"sidebarWeb"`
}

type preferencesDTO struct {
	Albums           prefAlbums           `json:"albums"`
	Cast             prefCast             `json:"cast"`
	Download         prefDownload         `json:"download"`
	EmailNotifications prefEmailNotif     `json:"emailNotifications"`
	Folders          prefFolders          `json:"folders"`
	Memories         prefMemories         `json:"memories"`
	People           prefPeople           `json:"people"`
	Purchase         prefPurchase         `json:"purchase"`
	Ratings          prefRatings          `json:"ratings"`
	RecentlyAdded    prefRecentlyAdded    `json:"recentlyAdded"`
	SharedLinks      prefSharedLinks      `json:"sharedLinks"`
	Tags             prefTags             `json:"tags"`
}

func defaultPreferences() preferencesDTO {
	return preferencesDTO{
		Albums:            prefAlbums{DefaultAssetOrder: "desc"},
		Cast:              prefCast{GCastEnabled: false},
		Download:          prefDownload{ArchiveSize: 100, IncludeEmbeddedVideos: true},
		EmailNotifications: prefEmailNotif{AlbumInvite: true, AlbumUpdate: true, Enabled: false},
		Folders:           prefFolders{Enabled: false, SidebarWeb: false},
		Memories:          prefMemories{Duration: 30, Enabled: true},
		People:            prefPeople{Enabled: true, MinimumFaces: 3, SidebarWeb: true},
		Purchase:          prefPurchase{HideBuyButtonUntil: "1970-01-01T00:00:00.000Z", ShowSupportBadge: true},
		Ratings:           prefRatings{Enabled: false},
		RecentlyAdded:     prefRecentlyAdded{SidebarWeb: true},
		SharedLinks:       prefSharedLinks{Enabled: true, SidebarWeb: true},
		Tags:              prefTags{Enabled: true, SidebarWeb: true},
	}
}

func (a *App) loadPreferences(userID string) preferencesDTO {
	p := defaultPreferences()
	var row UserPreferences
	if err := a.store.DB.First(&row, "user_id = ?", userID).Error; err == nil && row.Data != "" {
		// Best-effort: keep defaults for any field the stored blob omits.
		_ = unmarshalPreferences(row.Data, &p)
	}
	return p
}

func (a *App) handleAdminUserPreferences(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	id := c.Param("id")
	if err := a.store.DB.First(&User{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found", "statusCode": 404})
		return
	}
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, a.loadPreferences(id))
		return
	}
	// PUT: merge provided top-level sections over the stored preferences.
	existing := a.loadPreferences(id)
	var patch map[string]json.RawMessage
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid body", "statusCode": 400})
		return
	}
	merged := mergePreferences(patch, existing)
	data, err := json.Marshal(merged)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	upd := UserPreferences{UserID: id, Data: string(data)}
	a.store.DB.Save(&upd)
	c.JSON(http.StatusOK, merged)
}

func unmarshalPreferences(data string, p *preferencesDTO) error {
	return json.Unmarshal([]byte(data), p)
}

// mergePreferences overwrites whole top-level sections present in the patch
// over the base preferences (the official client sends complete sub-objects).
func mergePreferences(patch map[string]json.RawMessage, base preferencesDTO) preferencesDTO {
	out := base
	for key, raw := range patch {
		switch key {
		case "albums":
			_ = json.Unmarshal(raw, &out.Albums)
		case "cast":
			_ = json.Unmarshal(raw, &out.Cast)
		case "download":
			_ = json.Unmarshal(raw, &out.Download)
		case "emailNotifications":
			_ = json.Unmarshal(raw, &out.EmailNotifications)
		case "folders":
			_ = json.Unmarshal(raw, &out.Folders)
		case "memories":
			_ = json.Unmarshal(raw, &out.Memories)
		case "people":
			_ = json.Unmarshal(raw, &out.People)
		case "purchase":
			_ = json.Unmarshal(raw, &out.Purchase)
		case "ratings":
			_ = json.Unmarshal(raw, &out.Ratings)
		case "recentlyAdded":
			_ = json.Unmarshal(raw, &out.RecentlyAdded)
		case "sharedLinks":
			_ = json.Unmarshal(raw, &out.SharedLinks)
		case "tags":
			_ = json.Unmarshal(raw, &out.Tags)
		default:
			// Ignore unknown keys (e.g. "avatar") without erroring.
		}
	}
	return out
}
