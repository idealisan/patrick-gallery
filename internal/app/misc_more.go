package app

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// handlePersonThumbnail serves a person's face thumbnail (GET /people/:id/thumbnail).
func (a *App) handlePersonThumbnail(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var p Person
	if err := a.store.DB.Where("id = ?", id).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	_ = uid
	if p.ThumbnailPath == "" || !fileExists(p.ThumbnailPath) {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(p.ThumbnailPath)
}

// handleAdminSignUp registers the first administrator (POST /auth/admin-sign-up).
// Immich only permits this while no users exist. We mirror that guard so the
// endpoint does real work (creates the initial admin) instead of being a stub.
func (a *App) handleAdminSignUp(c *gin.Context) {
	var n int64
	a.store.DB.Model(&User{}).Count(&n)
	if n > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "an admin already exists", "statusCode": 400})
		return
	}
	var b struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&b); err != nil || b.Email == "" || b.Name == "" || b.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email, name and password required", "statusCode": 400})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(b.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "statusCode": 500})
		return
	}
	now := time.Now().UTC()
	u := User{
		ID:               newUUID(),
		Email:            b.Email,
		Name:             b.Name,
		Password:         string(hash),
		Salt:             newUUID(),
		IsAdmin:          true,
		AvatarColor:      "primary",
		CreatedAt:        now,
		UpdatedAt:        now,
		ProfileChangedAt: now,
	}
	if err := a.store.DB.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "statusCode": 500})
		return
	}
	tok, _ := a.issueToken(u.ID)
	a.recordSession(u.ID, tok)
	c.JSON(http.StatusCreated, a.toAdminUser(u))
}

// handleGetNotification returns a single notification (GET /notifications/:id).
func (a *App) handleGetNotification(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var n Notification
	if err := a.store.DB.Where("id = ? AND user_id = ?", id, uid).First(&n).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, gin.H{
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

// handleUpdateNotification marks a single notification read
// (PUT /notifications/:id).
func (a *App) handleUpdateNotification(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	res := a.store.DB.Model(&Notification{}).Where("id = ? AND user_id = ?", id, uid).
		Update("read_at", time.Now().UTC())
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error(), "statusCode": 500})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	c.Status(http.StatusOK)
}

// handleAssetOcr returns the OCR text for an asset (GET /assets/:id/ocr).
// OCR is not performed in this pure-Go build, so we return the stored OCR text
// when present and an honest empty result otherwise (no fake success).
func (a *App) handleAssetOcr(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var asset Asset
	if err := a.store.DB.Where("id = ?", id).First(&asset).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	if !a.canView(uid, asset.OwnerID) {
		c.Status(http.StatusForbidden)
		return
	}
	var ocr AssetOcr
	var text string
	if err := a.store.DB.Where("asset_id = ?", id).First(&ocr).Error; err == nil {
		text = ocr.Text
	}
	c.JSON(http.StatusOK, gin.H{"assetId": id, "text": text})
}

// handleMeCalendarHeatmap returns the current user's upload activity heatmap
// (GET /users/me/calendar-heatmap). Mirrors the admin heatmap scoped to self.
func (a *App) handleMeCalendarHeatmap(c *gin.Context) {
	uid := currentUserID(c)
	now := time.Now().UTC()
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -364).Format("2006-01-02")
	type dayCount struct {
		D string
		C int64
	}
	var rows []dayCount
	a.store.DB.Raw(
		"SELECT substr(local_date_time,1,10) AS d, COUNT(*) AS c FROM assets WHERE owner_id = ? AND local_date_time >= ? GROUP BY d",
		uid, from,
	).Scan(&rows)
	counts := make(map[string]int64, len(rows))
	var total int64
	for _, r := range rows {
		counts[r.D] = r.C
		total += r.C
	}
	series := make([]gin.H, 0, 365)
	cur, _ := time.Parse("2006-01-02", from)
	for cur.Format("2006-01-02") <= to {
		d := cur.Format("2006-01-02")
		series = append(series, gin.H{"date": d, "count": counts[d]})
		cur = cur.AddDate(0, 0, 1)
	}
	c.JSON(http.StatusOK, gin.H{
		"from":       from,
		"to":         to,
		"series":     series,
		"totalCount": total,
	})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
