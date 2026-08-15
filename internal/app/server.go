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
		// required by ServerAboutResponseDto
		"version":    a.cfg.CompatVersion,
		"licensed":   false,
		"versionUrl": "https://github.com/immich-app/immich",
		// optional but expected by clients
		"build":          "immich-go",
		"buildImage":     "immich-go",
		"buildImageUrl":  "https://github.com/immich-app/immich",
		"buildUrl":       "https://github.com/immich-app/immich",
		"repository":     "https://github.com/immich-app/immich",
		"repositoryUrl":  "https://github.com/immich-app/immich",
		"sourceCommit":   "immich-go",
		"sourceRef":      "main",
		"canUpdate":      false,
		"isDocker":       false,
		"isProd":         true,
		"externalDomain": domain,
		"baseUrl":        base,
	})
}
