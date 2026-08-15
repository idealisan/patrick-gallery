package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// compat_v3.go hardens compatibility with the official Immich v3.1.0 client.
// The v3.1.0 OpenAPI spec defines 254 method-paths; this file closes the
// safe, high-value gaps (HTTP-method mismatches, startup info endpoints the
// app polls, and graceful stubs for ML/sync surfaces) without pulling in the
// out-of-scope ML / multi-user backends (faces clustering, CLIP, OAuth,
// admin, memories, notifications, plugins, workflows, queues, sessions).
//
// Stub endpoints return the correct, empty DTO shape so the official clients
// proceed instead of erroring; they do NOT fake ML results.

// ---- startup / server-info endpoints the app polls ----

func (a *App) handleServerVersionCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"isAvailableUpdate": false, "isAllowAutoUpdate": false})
}

func (a *App) handleServerMediaTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"video": []string{"mp4", "mov", "avi", "mkv", "webm", "m4v"},
		"image": []string{"jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp"},
	})
}

func (a *App) handleServerStorage(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"diskAvailable":       0,
		"diskSize":            0,
		"diskUse":             0,
		"diskUsagePercentage": 0,
	})
}

func (a *App) handleServerApkLinks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"android": "", "ios": ""})
}

func (a *App) handleServerVersionHistory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"lastVersionCheck": nil, "versions": []any{}})
}

func (a *App) handleServerLicense(c *gin.Context) {
	if c.Request.Method == http.MethodPut {
		c.Status(http.StatusOK)
		return
	}
	c.JSON(http.StatusOK, gin.H{"activationKey": "", "licenseKey": "", "license": nil})
}

// ---- sync service (graceful stub) ----

func (a *App) handleSyncAck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"acqSequence": 0, "syncedAt": nil})
}

// handleSyncStream is a no-op delta stream. The official client opens it for
// incremental sync; with no ML/metadata deltas to push we return an empty
// response so the client proceeds (the Socket.IO channel carries live asset/
// album events instead).
func (a *App) handleSyncStream(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

// ---- people / faces (no ML backend) ----

func (a *App) handlePersonStatistics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"assets": 0})
}

func (a *App) handleFacesList(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

func (a *App) handleFaceGet(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"message": "no ML backend", "statusCode": 404})
}

func (a *App) handlePersonMerge(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (a *App) handlePersonReassign(c *gin.Context) {
	c.Status(http.StatusOK)
}

// ---- search helpers (extra surfaces) ----

func (a *App) handleSearchCities(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

func (a *App) handleSearchPlaces(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"places": []any{}, "recentPlaces": []any{}, "allPlaces": []any{}})
}

// ---- minor aliases / stubs ----

func (a *App) handleDownloadInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"size": 0})
}

func (a *App) handleDuplicatesResolve(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (a *App) handleDuplicatesUpdate(c *gin.Context) {
	c.Status(http.StatusOK)
}
