package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMapMarkersReturnsGeoTagged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app := newTestApp(t)

	var admin User
	if err := app.store.DB.First(&admin).Error; err != nil {
		t.Fatalf("no admin: %v", err)
	}

	now := time.Now().UTC()
	// two assets at (near) the same spot should cluster into one marker;
	// a third with no GPS should be excluded.
	a1 := Asset{ID: newUUID(), OwnerID: admin.ID, Type: "IMAGE", IsTrash: false, CreatedAt: now, UpdatedAt: now}
	a2 := Asset{ID: newUUID(), OwnerID: admin.ID, Type: "IMAGE", IsTrash: false, CreatedAt: now, UpdatedAt: now}
	a3 := Asset{ID: newUUID(), OwnerID: admin.ID, Type: "IMAGE", IsTrash: false, CreatedAt: now, UpdatedAt: now}
	if err := app.store.DB.Create(&a1).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.store.DB.Create(&a2).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.store.DB.Create(&a3).Error; err != nil {
		t.Fatal(err)
	}
	app.store.DB.Create(&Exif{ID: newUUID(), AssetID: a1.ID, Latitude: 48.85, Longitude: 2.35, City: "Paris", Country: "France"})
	app.store.DB.Create(&Exif{ID: newUUID(), AssetID: a2.ID, Latitude: 48.851, Longitude: 2.351, City: "Paris", Country: "France"})
	app.store.DB.Create(&Exif{ID: newUUID(), AssetID: a3.ID, Latitude: 0, Longitude: 0}) // excluded (0,0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(ctxUserID, admin.ID)
	app.handleMapMarkers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var markers []MapMarker
	if err := json.Unmarshal(w.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 clustered marker, got %d: %+v", len(markers), markers)
	}
	m := markers[0]
	if m.Count != 2 {
		t.Errorf("expected count 2, got %d", m.Count)
	}
	if m.City != "Paris" || m.Country != "France" {
		t.Errorf("expected Paris/France, got %q/%q", m.City, m.Country)
	}
	if m.AssetID == "" {
		t.Errorf("marker missing representative assetId")
	}
}
