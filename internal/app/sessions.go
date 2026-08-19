package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleSessionsList mirrors GET /api/sessions. Lists the current user's
// active sessions (the in-memory bus tracks them at login/token issuance).
func (a *App) handleSessionsList(c *gin.Context) {
	uid := currentUserID(c)
	var sessions []Session
	a.store.DB.Where("user_id = ?", uid).Order("created_at DESC").Find(&sessions)
	out := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, a.sessionToDTO(&s, s.ID == currentSessionID(c)))
	}
	c.JSON(http.StatusOK, out)
}

// handleSessionGet mirrors GET /api/sessions/:id.
func (a *App) handleSessionGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var s Session
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, a.sessionToDTO(&s, s.ID == currentSessionID(c)))
}

// handleSessionUpdate mirrors PUT /api/sessions/:id. Immich uses this to toggle
// the pending-sync-reset flag (and, on mobile, device metadata). We persist
// isPendingSyncReset (and deviceOS/deviceType/appVersion when provided).
func (a *App) handleSessionUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var s Session
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var b struct {
		IsPendingSyncReset *bool  `json:"isPendingSyncReset"`
		DeviceOS           string `json:"deviceOS"`
		DeviceType         string `json:"deviceType"`
		AppVersion         string `json:"appVersion"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.IsPendingSyncReset != nil {
		s.IsPendingSyncReset = *b.IsPendingSyncReset
	}
	if b.DeviceOS != "" {
		s.DeviceOS = b.DeviceOS
	}
	if b.DeviceType != "" {
		s.DeviceType = b.DeviceType
	}
	if b.AppVersion != "" {
		s.AppVersion = b.AppVersion
	}
	s.UpdatedAt = time.Now().UTC()
	a.store.DB.Save(&s)
	c.JSON(http.StatusOK, a.sessionToDTO(&s, s.ID == currentSessionID(c)))
}

// handleSessionDelete mirrors DELETE /api/sessions/:id. Ends a single session.
func (a *App) handleSessionDelete(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&Session{})
	c.Status(http.StatusOK)
}

// handleSessionsDeleteAll mirrors DELETE /api/sessions. Ends every other
// session for the user (effectively "log out everywhere").
func (a *App) handleSessionsDeleteAll(c *gin.Context) {
	uid := currentUserID(c)
	a.store.DB.Where("user_id = ?", uid).Delete(&Session{})
	c.Status(http.StatusOK)
}

// handleSessionLock mirrors POST /api/sessions/:id/lock. Applies a PIN/session
// lock so the device must re-authenticate (Immich's child-session lock). We
// record PinExpiresAt far in the future (until explicitly unlocked) and emit a
// session.lock event so connected clients re-prompt.
func (a *App) handleSessionLockByID(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var s Session
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	s.PinExpiresAt = &expiry
	a.store.DB.Save(&s)
	a.emit("on_session_update", map[string]any{"userId": uid, "action": "lock", "id": id})
	c.JSON(http.StatusOK, a.sessionToDTO(&s, s.ID == currentSessionID(c)))
}

func (a *App) sessionToDTO(s *Session, current bool) gin.H {
	out := gin.H{
		"id":                 s.ID,
		"userId":             s.UserID,
		"parentId":           s.ParentID,
		"deviceOS":           s.DeviceOS,
		"deviceType":         s.DeviceType,
		"appVersion":         s.AppVersion,
		"isPendingSyncReset": s.IsPendingSyncReset,
		"current":            current,
		"createdAt":          s.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":          s.UpdatedAt.UTC().Format(time.RFC3339),
		"expiresAt":          s.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if s.PinExpiresAt != nil {
		out["pinExpiresAt"] = s.PinExpiresAt.UTC().Format(time.RFC3339)
	} else {
		out["pinExpiresAt"] = nil
	}
	return out
}

// currentSessionID returns the session id bound to the caller's JWT, if any.
func currentSessionID(c *gin.Context) string {
	if v, ok := c.Get("sessionID"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
