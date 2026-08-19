package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

// handleStacksList returns the current user's stacks (GET /stacks). Mirrors the
// official contract: an array of stack objects each carrying an `assets` array
// (the non-primary members, in order) and the primary asset id.
func (a *App) handleStacksList(c *gin.Context) {
	uid := currentUserID(c)
	var stacks []Stack
	if err := a.store.DB.Where("owner_id = ?", uid).Order("created_at DESC").Find(&stacks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "statusCode": 500})
		return
	}
	out := make([]map[string]any, 0, len(stacks))
	for _, s := range stacks {
		var members []StackAsset
		a.store.DB.Where("stack_id = ?", s.ID).Order("\"order\" ASC").Find(&members)
		assets := make([]map[string]any, 0, len(members))
		for _, m := range members {
			var asset Asset
			if a.store.DB.Where("id = ?", m.AssetID).First(&asset).Error != nil {
				continue
			}
			assets = append(assets, gin.H{
				"id":          asset.ID,
				"duplicateId": nil,
				"type":        asset.Type,
			})
		}
		out = append(out, gin.H{
			"id":            s.ID,
			"primaryAssetId": s.PrimaryAssetID,
			"ownerId":       s.OwnerID,
			"createdAt":     s.CreatedAt,
			"updatedAt":     s.UpdatedAt,
			"assets":        assets,
		})
	}
	c.JSON(http.StatusOK, out)
}

// handleStackUpdate merges another asset into a stack as primary
// (PUT /stacks/:id). The official endpoint takes the target primary asset id and
// folds the existing stack's assets under it.
func (a *App) handleStackUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	var b struct {
		PrimaryAssetID string `json:"primaryAssetId"`
	}
	if err := c.ShouldBindJSON(&b); err != nil || b.PrimaryAssetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "primaryAssetId required", "statusCode": 400})
		return
	}
	var s Stack
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var primary Asset
	if a.store.DB.Where("id = ? AND owner_id = ?", b.PrimaryAssetID, uid).First(&primary).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "primary asset not found", "statusCode": 404})
		return
	}

	// The previous primary becomes a member.
	var members []StackAsset
	a.store.DB.Where("stack_id = ?", s.ID).Order("\"order\" ASC").Find(&members)
	// Remove any existing membership for the new primary.
	a.store.DB.Where("stack_id = ? AND asset_id = ?", s.ID, b.PrimaryAssetID).Delete(&StackAsset{})
	newOrder := 0
	// add old primary first
	a.store.DB.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&StackAsset{StackID: s.ID, AssetID: s.PrimaryAssetID, Order: newOrder})
	newOrder++
	for _, m := range members {
		if m.AssetID == b.PrimaryAssetID {
			continue
		}
		a.store.DB.Clauses(clause.OnConflict{UpdateAll: true}).
			Create(&StackAsset{StackID: s.ID, AssetID: m.AssetID, Order: newOrder})
		newOrder++
	}
	s.PrimaryAssetID = b.PrimaryAssetID
	a.store.DB.Save(&s)
	c.JSON(http.StatusOK, gin.H{"id": s.ID, "primaryAssetId": s.PrimaryAssetID, "ownerId": s.OwnerID})
}

// handleStackAssetGet returns a single member asset of a stack
// (GET /stacks/:id/assets/:assetId).
func (a *App) handleStackAssetGet(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	assetID := c.Param("assetId")
	var s Stack
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var m StackAsset
	if err := a.store.DB.Where("stack_id = ? AND asset_id = ?", s.ID, assetID).First(&m).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	var asset Asset
	if a.store.DB.Where("id = ?", assetID).First(&asset).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found", "statusCode": 404})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": asset.ID, "type": asset.Type, "duplicateId": nil})
}

// handleStackAssetUpdate removes an asset from a stack (PUT /stacks/:id/assets/:assetId).
// Per the official contract this unstacks the single asset (it becomes its own
// primary, leaving the remaining stack intact).
func (a *App) handleStackAssetUpdate(c *gin.Context) {
	uid := currentUserID(c)
	id := c.Param("id")
	assetID := c.Param("assetId")
	var s Stack
	if err := a.store.DB.Where("id = ? AND owner_id = ?", id, uid).First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	if a.store.DB.Where("stack_id = ? AND asset_id = ?", s.ID, assetID).Delete(&StackAsset{}).RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
		return
	}
	// If it was the primary, promote the first remaining member to primary.
	if s.PrimaryAssetID == assetID {
		var remaining []StackAsset
		a.store.DB.Where("stack_id = ?", s.ID).Order("\"order\" ASC").Find(&remaining)
		if len(remaining) > 0 {
			s.PrimaryAssetID = remaining[0].AssetID
		} else {
			// No members left → dissolve the stack.
			a.store.DB.Delete(&s)
			c.Status(http.StatusNoContent)
			return
		}
		a.store.DB.Save(&s)
	}
	c.Status(http.StatusNoContent)
}

// handleStackAssetDelete is an alias of the unstack action
// (DELETE /stacks/:id/assets/:assetId).
func (a *App) handleStackAssetDelete(c *gin.Context) {
	a.handleStackAssetUpdate(c)
}
