package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) handleAbout(c *gin.Context) {
	domain := a.cfg.ExternalDomain
	base := domain
	if base == "" {
		base = "http://" + a.cfg.Host + ":" + itoa(a.cfg.Port)
	}
	c.JSON(http.StatusOK, gin.H{
		"version":   "1.0.0-go",
		"build":     gin.H{"releaseTrack": "nightly", "repo": "immich-go"},
		"licensed":  false,
		"canUpdate": false,
		"isDocker":  false,
		"isProd":    true,
		"externalDomain": domain,
		"baseUrl":   base,
	})
}
