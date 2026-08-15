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

// handleListNotifications returns the current user's notifications as a
// NotificationDto[] (Immich: GET /api/notifications?unread=true). The private
// instance has no notification generator, so this returns the real (usually
// empty) rows — the honest state that lets the official web client proceed.
func (a *App) handleListNotifications(c *gin.Context) {
	uid := currentUserID(c)
	q := a.store.DB.Where("user_id = ?", uid)
	if c.Query("unread") == "true" {
		q = q.Where("read_at IS NULL")
	}
	var notes []Notification
	if err := q.Order("created_at DESC").Find(&notes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	out := make([]gin.H, 0, len(notes))
	for _, n := range notes {
		out = append(out, gin.H{
			"id":          n.ID,
			"userId":      n.UserID,
			"type":        n.Type,
			"level":       n.Level,
			"title":       n.Title,
			"description": n.Description,
			"readAt":      n.ReadAt,
			"createdAt":   n.CreatedAt,
			"updatedAt":   n.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleUpdateNotifications applies a bulk action (e.g. mark-all-read) to the
// current user's notifications (Immich: PUT /api/notifications).
func (a *App) handleUpdateNotifications(c *gin.Context) {
	uid := currentUserID(c)
	var body struct {
		Action string `json:"action"`
	}
	_ = c.ShouldBindJSON(&body)
	now := time.Now().UTC()
	switch body.Action {
	case "read-all":
		a.store.DB.Model(&Notification{}).Where("user_id = ? AND read_at IS NULL", uid).
			Update("read_at", now)
	case "archive-all", "remove-all":
		a.store.DB.Where("user_id = ?", uid).Delete(&Notification{})
	}
	c.Status(http.StatusOK)
}

// handleDeleteNotification removes a single notification (Immich:
// DELETE /api/notifications/:id).
func (a *App) handleDeleteNotification(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&Notification{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.Status(http.StatusNoContent)
}

