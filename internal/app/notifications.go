package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleNotificationRegister persists a mobile push device token for the
// current user (Immich: POST /api/notifications). immich-go has no external
// push provider in its private-LAN scope, so tokens are stored for
// data-model completeness and the call returns success without dispatching a
// push. The request body is JSON ({deviceToken, platform}); form values are
// accepted as a fallback for web clients.
func (a *App) handleNotificationRegister(c *gin.Context) {
	uid := currentUserID(c)
	var body struct {
		DeviceToken string `json:"deviceToken"`
		Platform    string `json:"platform"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.DeviceToken = c.PostForm("deviceToken")
		body.Platform = c.PostForm("platform")
	}
	if body.DeviceToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "deviceToken required", "statusCode": 400})
		return
	}

	now := time.Now().UTC()
	var existing NotificationToken
	if err := a.store.DB.Where("user_id = ? AND device_token = ?", uid, body.DeviceToken).First(&existing).Error; err == nil {
		existing.Platform = body.Platform
		existing.UpdatedAt = now
		if err := a.store.DB.Save(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
			return
		}
	} else {
		if err := a.store.DB.Create(&NotificationToken{
			ID:          newUUID(),
			UserID:      uid,
			DeviceToken: body.DeviceToken,
			Platform:    body.Platform,
			CreatedAt:   now,
			UpdatedAt:   now,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"status": "ok"})
}

// handleNotificationRemove deletes the current user's registered device tokens
// (Immich: DELETE /api/notifications).
func (a *App) handleNotificationRemove(c *gin.Context) {
	uid := currentUserID(c)
	if err := a.store.DB.Where("user_id = ?", uid).Delete(&NotificationToken{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
