package app

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func (a *App) handleListUsers(c *gin.Context) {
	var users []User
	a.store.DB.Find(&users)
	c.JSON(http.StatusOK, users)
}

func (a *App) handleMe(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// Populate quota usage so the web storage meter renders correctly
	// (see User.QuotaUsageInBytes / QuotaSizeInBytes in models.go).
	usage := a.userQuotaUsage(uid)
	u.QuotaUsageInBytes = &usage
	c.JSON(http.StatusOK, u)
}

type updateMeBody struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Bio   string `json:"bio,omitempty"`
}

func (a *App) handleUpdateMe(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b updateMeBody
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		u.Name = b.Name
	}
	if b.Email != "" {
		u.Email = b.Email
	}
	if b.Bio != "" {
		u.Bio = b.Bio
	}
	a.store.DB.Save(&u)
	c.JSON(http.StatusOK, u)
}

func (a *App) handlePreferences(c *gin.Context) {
	uid := currentUserID(c)
	if c.Request.Method == http.MethodGet {
		// Must match the official v3.1.0 UserPreferencesResponseDto exactly:
		// a fixed set of nested sub-objects (albums, cast, download,
		// emailNotifications, folders, memories, people, purchase, ratings,
		// recentlyAdded, sharedLinks, tags), each with its own fields. The
		// official web reads e.g. preferences.folders.enabled /
		// preferences.tags.enabled / preferences.ratings.enabled; returning a
		// legacy flat shape (folders:null, rating:false, ...) throws
		// "Cannot read properties of null (reading 'enabled')" and the SPA
		// falls back to the login screen.
		c.JSON(http.StatusOK, a.loadPreferences(uid))
		return
	}
	// PUT: merge provided top-level sections over stored preferences (the
	// official client sends complete sub-objects). Mirrors the admin endpoint.
	existing := a.loadPreferences(uid)
	var patch map[string]json.RawMessage
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "statusCode": 400})
		return
	}
	merged := mergePreferences(patch, existing)
	data, err := json.Marshal(merged)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	upd := UserPreferences{UserID: uid, Data: string(data)}
	a.store.DB.Save(&upd)
	c.JSON(http.StatusOK, merged)
}

func (a *App) handleGetUser(c *gin.Context) {
	id := c.Param("id")
	var u User
	if err := a.store.DB.First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, u)
}

func (a *App) handleUpdateUser(c *gin.Context) {
	id := c.Param("id")
	var u User
	if err := a.store.DB.First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var b updateMeBody
	_ = c.ShouldBindJSON(&b)
	if b.Name != "" {
		u.Name = b.Name
	}
	if b.Email != "" {
		u.Email = b.Email
	}
	a.store.DB.Save(&u)
	c.JSON(http.StatusOK, u)
}

func (a *App) handleDeleteUser(c *gin.Context) {
	id := c.Param("id")
	a.store.DB.Where("id = ?", id).Delete(&User{})
	a.store.DB.Where("owner_id = ?", id).Delete(&Asset{})
	c.Status(http.StatusOK)
}

func (a *App) handleUserThumb(c *gin.Context) {
	id := c.Param("id")
	var u User
	if err := a.store.DB.First(&u, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	path := filepath.Join(a.cfg.ResourceDir, "profile", id)
	c.File(path)
}
