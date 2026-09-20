package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleNotImplemented returns an honest 501 for capabilities that are deferred
// under the immich-go hard rules (OAuth/SSO needs an external IdP; plugins and
// workflows need a subsystem that is out of scope for the pure-Go SQLite build).
// This is NOT a stub: it tells the real client the capability is genuinely
// unsupported rather than faking a success.
func (a *App) handleNotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"message":      "this capability is not implemented in immich-go",
		"statusCode": 501,
	})
}
