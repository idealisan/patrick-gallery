package app

import (
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
		c.JSON(http.StatusOK, gin.H{
			"id":                    uid,
			"email":                 "",
			"name":                  "",
			"avatarColor":           "primary",
			"memories":              gin.H{"enabled": true},
			"people":                gin.H{"enabled": true},
			"rating":                false,
			"theme":                 "system",
			"album":                 nil,
			"library":               nil,
			"tags":                  nil,
			"folders":               nil,
			"sharedLinks":           nil,
			"map":                   gin.H{"enabled": false, "reverseGeocoding": false},
			"jobOffset":             0,
			"stack":                 gin.H{"enabled": true},
			"search":                gin.H{"enabled": true},
			"sharedAlbums":          nil,
			"assetAdditionalInfo":   nil,
			"language":              "en-US",
			"download":              gin.H{"includeEmbeddedVideo": true},
			"trash":                 gin.H{"enabled": true, "days": 30},
			"oauthButtonColor":      nil,
			"foldersEnabled":        false,
			"archiveOnTimeline":     false,
			"duplicateDetection":    false,
			"rawOverlay":            false,
			"similarityDetection":   false,
			"downloadArchiveSize":   100,
			"thumbnailCacheEnabled": true,
		})
		return
	}
	// PUT: accept and echo back (scaffold stores only basic prefs on user row)
	var b map[string]interface{}
	_ = c.ShouldBindJSON(&b)
	c.JSON(http.StatusOK, b)
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
