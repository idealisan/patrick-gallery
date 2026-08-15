package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleOAuthConfig reports whether OAuth login is enabled. immich-go is a
// private-LAN, single-credential port with no external identity provider, so
// OAuth is always disabled. This endpoint exists so official clients probing
// /api/oauth/config do not receive a 404 (the mobile app issues this request
// during onboarding).
func (a *App) handleOAuthConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":              false,
		"passwordLoginEnabled": true,
	})
}
