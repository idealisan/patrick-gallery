package app

import (
	"time"

	"gorm.io/gorm"
)

// All primary keys are UUID strings (matching Immich). Timestamps are stored
// natively and serialised to RFC3339 like the original API.

type User struct {
	ID                  string         `gorm:"primaryKey;type:text" json:"id"`
	Email               string         `gorm:"type:text" json:"email"`
	Name                string         `gorm:"type:text" json:"name"`
	Password            string         `gorm:"type:text" json:"-"`
	Salt                string         `gorm:"type:text" json:"-"`
	IsAdmin             bool           `json:"isAdmin"`
	ShouldChangePassword bool          `json:"shouldChangePassword"`
	AvatarColor         string         `gorm:"type:text" json:"avatarColor"`
	StorageLabel        string         `gorm:"type:text" json:"storageLabel"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt  `gorm:"index" json:"-"`
	// profile / preferences (kept inline for the scaffold)
	Bio                 string         `gorm:"type:text" json:"bio,omitempty"`
	IsEmailActive       bool           `json:"isEmailActive,omitempty"`
}

type Asset struct {
	ID               string        `gorm:"primaryKey;type:text" json:"id"`
	DeviceAssetId    string        `gorm:"type:text" json:"deviceAssetId"`
	DeviceId         string        `gorm:"type:text" json:"deviceId"`
	OwnerID          string        `gorm:"index;type:text" json:"ownerId"`
	Type             string        `gorm:"type:text" json:"type"` // IMAGE | VIDEO
	OriginalPath     string        `gorm:"type:text" json:"originalPath"`
	OriginalFileName string        `gorm:"type:text" json:"originalFileName"`
	ResizePath       string        `gorm:"type:text" json:"resizePath"`
	EncodedVideoPath string        `gorm:"type:text" json:"encodedVideoPath"`
	Checksum         string        `gorm:"type:text;index" json:"checksum"`
	FileCreatedAt    time.Time     `json:"fileCreatedAt"`
	FileModifiedAt   time.Time     `json:"fileModifiedAt"`
	LocalDateTime    time.Time     `json:"localDateTime"`
	Duration         string        `gorm:"type:text" json:"duration"`
	IsFavorite       bool          `json:"isFavorite"`
	IsArchived       bool          `json:"isArchived"`
	IsTrash          bool          `json:"isTrash"`
	IsExternal       bool          `json:"isExternal"`
	LibraryId        string        `gorm:"type:text" json:"libraryId"`
	HasThumbnail     bool          `json:"hasThumbnail"`
	ExifID          string        `gorm:"type:text" json:"exifId"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type Exif struct {
	ID               string  `gorm:"primaryKey;type:text" json:"id"`
	AssetID          string  `gorm:"index;type:text" json:"assetId"`
	Make             string  `gorm:"type:text" json:"make,omitempty"`
	Model            string  `gorm:"type:text" json:"model,omitempty"`
	DateTimeOriginal *string `gorm:"type:text" json:"dateTimeOriginal,omitempty"`
	ExposureTime     string  `gorm:"type:text" json:"exposureTime,omitempty"`
	FNumber          float64 `json:"fNumber,omitempty"`
	ISO              int     `json:"iso,omitempty"`
	FocalLength      float64 `json:"focalLength,omitempty"`
	Latitude         float64 `json:"latitude,omitempty"`
	Longitude        float64 `json:"longitude,omitempty"`
	City             string  `gorm:"type:text" json:"city,omitempty"`
	Country          string  `gorm:"type:text" json:"country,omitempty"`
	Description      string  `gorm:"type:text" json:"description,omitempty"`
	Orientation      *int    `json:"orientation,omitempty"`
}

type Album struct {
	ID                     string        `gorm:"primaryKey;type:text" json:"id"`
	OwnerID                string        `gorm:"index;type:text" json:"ownerId"`
	AlbumName              string        `gorm:"type:text" json:"albumName"`
	Description            string        `gorm:"type:text" json:"description,omitempty"`
	AlbumThumbnailAssetId  string        `gorm:"type:text" json:"albumThumbnailAssetId,omitempty"`
	IsActivityEnabled      bool          `json:"isActivityEnabled,omitempty"`
	CreatedAt              time.Time     `json:"createdAt"`
	UpdatedAt              time.Time     `json:"updatedAt"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"-"`
}

// AlbumAsset is the join table (assets in an album, ordered).
type AlbumAsset struct {
	AlbumID   string    `gorm:"primaryKey;type:text" json:"albumId"`
	AssetID   string    `gorm:"primaryKey;type:text" json:"assetId"`
	CreatedAt time.Time `json:"createdAt"`
	Order     int       `json:"order,omitempty"`
}

type Library struct {
	ID           string        `gorm:"primaryKey;type:text" json:"id"`
	OwnerID      string        `gorm:"index;type:text" json:"ownerId"`
	Name         string        `gorm:"type:text" json:"name"`
	Type         string        `gorm:"type:text" json:"type"` // UPLOAD | EXTERNAL
	ImportPaths  string        `gorm:"type:text" json:"importPaths,omitempty"`
	ExcludedPaths string       `gorm:"type:text" json:"excludedPaths,omitempty"`
	Watched      bool          `json:"watched,omitempty"`
	Status       string        `gorm:"type:text" json:"status,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Partner struct {
	SharedByID   string `gorm:"primaryKey;type:text" json:"sharedById"`
	SharedWithID string `gorm:"primaryKey;type:text" json:"sharedWithId"`
}

type Tag struct {
	ID     string `gorm:"primaryKey;type:text" json:"id"`
	UserID string `gorm:"index;type:text" json:"userId"`
	Name   string `gorm:"type:text" json:"name"`
	Type   string `gorm:"type:text" json:"type,omitempty"`
	Color  string `gorm:"type:text" json:"color,omitempty"`
}

type AssetTag struct {
	AssetID string `gorm:"primaryKey;type:text" json:"assetId"`
	TagID   string `gorm:"primaryKey;type:text" json:"tagId"`
}

type Person struct {
	ID            string `gorm:"primaryKey;type:text" json:"id"`
	Name          string `gorm:"type:text" json:"name"`
	ThumbnailPath string `gorm:"type:text" json:"thumbnailPath,omitempty"`
	IsHidden      bool   `json:"isHidden,omitempty"`
}

type Activity struct {
	ID      string    `gorm:"primaryKey;type:text" json:"id"`
	AssetID string    `gorm:"type:text" json:"assetId,omitempty"`
	AlbumID string    `gorm:"type:text" json:"albumId,omitempty"`
	UserID  string    `gorm:"type:text" json:"userId"`
	Comment string    `gorm:"type:text" json:"comment"`
	CreatedAt time.Time `json:"createdAt"`
}

type SharedLink struct {
	ID        string     `gorm:"primaryKey;type:text" json:"id"`
	Key       string     `gorm:"type:text" json:"key"`
	Type      string     `gorm:"type:text" json:"type"` // ALBUM | INDIVIDUAL
	AssetID   string     `gorm:"type:text" json:"albumId,omitempty"`
	AlbumID   string     `gorm:"type:text" json:"albumId,omitempty"`
	UserID    string     `gorm:"type:text" json:"userId"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type ApiKey struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index;type:text" json:"userId"`
	Name      string    `gorm:"type:text" json:"name"`
	Key       string    `gorm:"type:text" json:"-"` // hashed
	CreatedAt time.Time `json:"createdAt"`
}

// SystemConfig is a singleton row holding global settings (id is fixed "singleton").
type SystemConfig struct {
	ID                 string `gorm:"primaryKey;type:text" json:"id"`
	LoginRequired      bool   `json:"loginRequired"`
	IsPublic           bool   `json:"isPublic"`
	ExternalDomain     string `gorm:"type:text" json:"externalDomain,omitempty"`
	NewPasswordRequired bool  `json:"newPasswordRequired,omitempty"`
}

func (User) TableName() string         { return "users" }
func (Asset) TableName() string        { return "assets" }
func (Exif) TableName() string         { return "exif" }
func (Album) TableName() string        { return "albums" }
func (AlbumAsset) TableName() string   { return "albums_assets_assets" }
func (Library) TableName() string      { return "libraries" }
func (Partner) TableName() string      { return "partners" }
func (Tag) TableName() string          { return "tags" }
func (AssetTag) TableName() string     { return "tags_assets" }
func (Person) TableName() string       { return "person" }
func (Activity) TableName() string     { return "activity" }
func (SharedLink) TableName() string   { return "shared_links" }
func (ApiKey) TableName() string       { return "api_keys" }
func (SystemConfig) TableName() string { return "system_config" }
