package app

import (
	"time"

	"gorm.io/gorm"
)

// All primary keys are UUID strings (matching Immich). Timestamps are stored
// natively and serialised to RFC3339 like the original API.

type User struct {
	ID                   string         `gorm:"primaryKey;type:text" json:"id"`
	Email                string         `gorm:"type:text" json:"email"`
	Name                 string         `gorm:"type:text" json:"name"`
	Password             string         `gorm:"type:text" json:"-"`
	Salt                 string         `gorm:"type:text" json:"-"`
	IsAdmin              bool           `json:"isAdmin"`
	ShouldChangePassword bool           `json:"shouldChangePassword"`
	AvatarColor          string         `gorm:"type:text" json:"avatarColor"`
	StorageLabel         string         `gorm:"type:text" json:"storageLabel"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
	// profile / preferences (kept inline for the scaffold)
	Bio           string `gorm:"type:text" json:"bio,omitempty"`
	IsEmailActive bool   `json:"isEmailActive,omitempty"`

	// admin-managed fields (mirror Immich's UserAdminResponseDto)
	PinCode          string `gorm:"type:text" json:"-"`
	QuotaSizeInBytes *int64 `gorm:"type:bigint" json:"quotaSizeInBytes"`
	// QuotaUsageInBytes is computed per-request (not persisted). It MUST be
	// present (null when no usage) so the web's `quotaSizeInBytes !== null`
	// check evaluates correctly; omitting it yields JS `undefined` which the
	// web treats as "has quota" and renders the storage meter as NaN.
	QuotaUsageInBytes *int64    `gorm:"-" json:"quotaUsageInBytes"`
	ProfileImagePath  string    `gorm:"type:text" json:"profileImagePath"`
	ProfileChangedAt  time.Time `json:"profileChangedAt"`
	// OAuthId is the external provider subject when the account was created
	// via OAuth; local accounts have an empty string. It MUST be serialized
	// (no omitempty) because the official v3.1.0 mobile client's
	// UserAdminResponseDto declares oauthId as a non-nullable string — a
	// missing/omitted field makes the openapi-generated fromJson crash with
	// "Null check operator used on a null value" and aborts login.
	OAuthId string `gorm:"type:text;default:''" json:"oauthId"`
	// Status mirrors Immich's UserStatus enum (active|removing|deleted). It is
	// serialized unconditionally: the mobile client's UserAdminResponseDto
	// requires a valid, non-null status, and an empty/unknown value throws.
	Status string `gorm:"type:text;default:active" json:"status"`
}

type Asset struct {
	ID               string         `gorm:"primaryKey;type:text" json:"id"`
	DeviceAssetId    string         `gorm:"type:text" json:"deviceAssetId"`
	DeviceId         string         `gorm:"type:text" json:"deviceId"`
	OwnerID          string         `gorm:"index;type:text" json:"ownerId"`
	Type             string         `gorm:"type:text" json:"type"` // IMAGE | VIDEO
	OriginalPath     string         `gorm:"type:text" json:"originalPath"`
	OriginalFileName string         `gorm:"type:text" json:"originalFileName"`
	ResizePath       string         `gorm:"type:text" json:"resizePath"`
	EncodedVideoPath string         `gorm:"type:text" json:"encodedVideoPath"`
	Checksum         string         `gorm:"type:text;index" json:"checksum"`
	FileCreatedAt    time.Time      `json:"fileCreatedAt"`
	FileModifiedAt   time.Time      `json:"fileModifiedAt"`
	LocalDateTime    time.Time      `json:"localDateTime"`
	Duration         string         `gorm:"type:text" json:"duration"`
	IsFavorite       bool           `json:"isFavorite"`
	IsArchived       bool           `json:"isArchived"`
	IsTrash          bool           `json:"isTrash"`
	TrashedAt        *time.Time     `gorm:"type:datetime" json:"-"`
	IsExternal       bool           `json:"isExternal"`
	LibraryId        string         `gorm:"type:text" json:"libraryId"`
	LivePhotoVideoID string         `gorm:"type:text" json:"-"`
	PersonID         string         `gorm:"type:text;index" json:"personId,omitempty"`
	Width            int            `gorm:"type:int" json:"-"`
	Height           int            `gorm:"type:int" json:"-"`
	Thumbhash        string         `gorm:"type:text" json:"-"`
	HasThumbnail     bool           `json:"hasThumbnail"`
	Size             int64          `json:"-"`
	ExifID           string         `gorm:"type:text" json:"exifId"`
	LastPlayedAt     *time.Time     `gorm:"type:datetime" json:"-"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
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
	State            string  `gorm:"type:text" json:"state,omitempty"`
	Description      string  `gorm:"type:text" json:"description,omitempty"`
	Orientation      *int    `json:"orientation,omitempty"`
	Rating           *int    `json:"rating,omitempty"`
}

// AssetOcr stores the normalized OCR output for an asset. Words are retained
// as JSON so providers can preserve coordinates/confidence without coupling
// the database schema to one engine.
type AssetOcr struct {
	AssetID   string    `gorm:"primaryKey;type:text" json:"assetId"`
	Text      string    `gorm:"type:text" json:"text"`
	WordsJSON string    `gorm:"type:text" json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AssetML struct {
	AssetID     string    `gorm:"primaryKey;type:text" json:"assetId"`
	Description string    `gorm:"type:text" json:"description"`
	LabelsJSON  string    `gorm:"type:text" json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// NotificationToken stores a mobile push device token registered by a client
// via POST /api/notifications. immich-go has no external push provider in its
// private-LAN scope, so these are persisted for data-model completeness only;
// nothing is dispatched. One row is kept per (user, device token).
type NotificationToken struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	UserID      string    `gorm:"index;type:text" json:"userId"`
	DeviceToken string    `gorm:"type:text" json:"deviceToken"`
	Platform    string    `gorm:"type:text" json:"platform"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Notification models a user-facing notification (system/asset notifications).
// immich-go is a private-LAN single-user instance with no notification
// generator, so this table is normally empty — GET /api/notifications returns
// the real (empty) rows rather than a faked constant, which is the honest
// state and satisfies the official web client's poll after login.
type Notification struct {
	ID          string     `gorm:"primaryKey;type:text" json:"id"`
	UserID      string     `gorm:"index;type:text" json:"userId"`
	Type        string     `gorm:"type:text" json:"type"`
	Level       string     `gorm:"type:text" json:"level"` // info | warning | error
	Title       string     `gorm:"type:text" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Metadata    string     `gorm:"type:text" json:"metadata,omitempty"`
	ReadAt      *time.Time `json:"readAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type Album struct {
	ID                    string         `gorm:"primaryKey;type:text" json:"id"`
	OwnerID               string         `gorm:"index;type:text" json:"ownerId"`
	AlbumName             string         `gorm:"type:text" json:"albumName"`
	Description           string         `gorm:"type:text" json:"description,omitempty"`
	AlbumThumbnailAssetId string         `gorm:"type:text" json:"albumThumbnailAssetId,omitempty"`
	IsActivityEnabled     bool           `json:"isActivityEnabled,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// AlbumAsset is the join table (assets in an album, ordered).
type AlbumAsset struct {
	AlbumID   string    `gorm:"primaryKey;type:text" json:"albumId"`
	AssetID   string    `gorm:"primaryKey;type:text" json:"assetId"`
	CreatedAt time.Time `json:"createdAt"`
	Order     int       `json:"order,omitempty"`
}

// AlbumUser grants a user access to an album (in-album sharing).
type AlbumUser struct {
	AlbumID string `gorm:"primaryKey;type:text" json:"albumId"`
	UserID  string `gorm:"primaryKey;type:text" json:"userId"`
	Role    string `gorm:"type:text" json:"role"` // editor | viewer
}

type Library struct {
	ID            string         `gorm:"primaryKey;type:text" json:"id"`
	OwnerID       string         `gorm:"index;type:text" json:"ownerId"`
	Name          string         `gorm:"type:text" json:"name"`
	Type          string         `gorm:"type:text" json:"type"` // UPLOAD | EXTERNAL
	ImportPaths   string         `gorm:"type:text" json:"importPaths,omitempty"`
	ExcludedPaths string         `gorm:"type:text" json:"excludedPaths,omitempty"`
	Watched       bool           `json:"watched,omitempty"`
	Status        string         `gorm:"type:text" json:"status,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Partner struct {
	SharedByID   string `gorm:"primaryKey;type:text" json:"sharedById"`
	SharedWithID string `gorm:"primaryKey;type:text" json:"sharedWithId"`
	InTimeline   bool   `gorm:"type:bool;default:false" json:"inTimeline,omitempty"`
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
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	AssetID   string    `gorm:"type:text" json:"assetId,omitempty"`
	AlbumID   string    `gorm:"type:text" json:"albumId,omitempty"`
	UserID    string    `gorm:"type:text" json:"userId"`
	Comment   string    `gorm:"type:text" json:"comment"`
	CreatedAt time.Time `json:"createdAt"`
}

type SharedLink struct {
	ID            string     `gorm:"primaryKey;type:text" json:"id"`
	Key           string     `gorm:"type:text" json:"key"`
	Type          string     `gorm:"type:text" json:"type"` // ALBUM | INDIVIDUAL
	AssetID       string     `gorm:"type:text" json:"assetId,omitempty"`
	AlbumID       string     `gorm:"type:text" json:"albumId,omitempty"`
	UserID        string     `gorm:"type:text" json:"userId"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	AllowDownload bool       `json:"allowDownload"`
	AllowUpload   bool       `json:"allowUpload"`
	Description   string     `gorm:"type:text" json:"description,omitempty"`
	Password      string     `gorm:"type:text" json:"password,omitempty"` // hashed
	ShowMetadata  bool       `json:"showMetadata"`
	Slug          string     `gorm:"type:text" json:"slug,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type ApiKey struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	UserID    string    `gorm:"index;type:text" json:"userId"`
	Name      string    `gorm:"type:text" json:"name"`
	Key       string    `gorm:"type:text" json:"-"` // hashed
	CreatedAt time.Time `json:"createdAt"`
}

// DuplicateResolution records that the user chose `AssetID` as the keeper and
// `DuplicateID` as the hidden duplicate (set via POST /duplicates/resolve). It
// lets GET /assets/duplicates stop re-surfacing resolved pairs.
type DuplicateResolution struct {
	AssetID     string    `gorm:"primaryKey;type:text" json:"assetId"`
	DuplicateID string    `gorm:"primaryKey;type:text" json:"duplicateId"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SyncState records the last acknowledged sync sequence per user so the
// /sync/stream delta feed can advance instead of always re-emitting everything.
type SyncState struct {
	UserID       string    `gorm:"primaryKey;type:text" json:"userId"`
	LastAckType  string    `gorm:"type:text" json:"lastAckType"`
	LastAckToken string    `gorm:"type:text" json:"lastAckToken"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// SystemConfig is a singleton row holding global settings (id is fixed "singleton").
type SystemConfig struct {
	ID                  string `gorm:"primaryKey;type:text" json:"id"`
	LoginRequired       bool   `json:"loginRequired"`
	IsPublic            bool   `json:"isPublic"`
	ExternalDomain      string `gorm:"type:text" json:"externalDomain,omitempty"`
	NewPasswordRequired bool   `json:"newPasswordRequired,omitempty"`
	TrashDays           int    `json:"trashDays"`
	Onboarded           bool   `json:"onboarded"`
	// JWTSecret is the per-instance HMAC key used to sign auth tokens. It is
	// generated once on first run and persisted here (never exposed via the
	// API). A constant compile-time default would let tokens minted by any
	// instance stay valid everywhere; a per-instance secret means a token
	// stored in a browser from a previous deployment is rejected, so the
	// login screen is shown instead of silently resuming a stale session.
	JWTSecret string `gorm:"type:text" json:"-"`

	// ConfigJSON stores the full nested SystemConfigDto (the body of
	// PUT /system-config) as a JSON blob. This makes the admin system-settings
	// pages round-trip: every block (ffmpeg/image/job/oauth/...) the web edits
	// is persisted verbatim and echoed back on GET, instead of resetting to
	// compile-time defaults on every update. Fields immich-go also mirrors into
	// flat columns (LoginRequired/ExternalDomain/...) stay authoritative and
	// are merged over the blob at read time.
	ConfigJSON string `gorm:"type:text" json:"-"`
}

// Session records an issued auth token so an admin can list a user's active
// sessions via /admin/users/:id/sessions, and so the mobile client's
// session-lock / child-session features have real server-side state. The
// (hashed) request token links a JWT back to its session row; PIN lock state
// lives in PinExpiresAt.
type Session struct {
	ID                 string     `gorm:"primaryKey;type:text" json:"id"`
	UserID             string     `gorm:"index;type:text" json:"userId"`
	ParentID           string     `gorm:"type:text" json:"parentId,omitempty"`
	Token              string     `gorm:"type:text" json:"-"` // sha256 of the issued JWT
	DeviceOS           string     `gorm:"type:text" json:"deviceOS"`
	DeviceType         string     `gorm:"type:text" json:"deviceType"`
	AppVersion         string     `gorm:"type:text" json:"appVersion,omitempty"`
	IsPendingSyncReset bool       `json:"isPendingSyncReset"`
	PinExpiresAt       *time.Time `json:"pinExpiresAt,omitempty"`
	Current            bool       `json:"current"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
}

// UserPreferences persists a user's UI preferences as a JSON blob (keyed by
// user id). A missing row falls back to defaults in the handler.
type UserPreferences struct {
	UserID string `gorm:"primaryKey;type:text" json:"userId"`
	Data   string `gorm:"type:text" json:"data"`
}

func (User) TableName() string       { return "users" }
func (Asset) TableName() string      { return "assets" }
func (Exif) TableName() string       { return "exif" }
func (Album) TableName() string      { return "albums" }
func (AlbumAsset) TableName() string { return "albums_assets_assets" }
func (AlbumUser) TableName() string  { return "albums_users_album" }
func (Library) TableName() string    { return "libraries" }
func (Partner) TableName() string    { return "partners" }
func (Tag) TableName() string        { return "tags" }
func (AssetTag) TableName() string   { return "tags_assets" }

// AssetEdit stores a non-destructive edit applied to an asset (crop / rotate /
// mirror). Only action + parameters are persisted (no metadata fields); the
// client applies them to the original when rendering. Parameters is a JSON
// blob whose shape depends on the action.
type AssetEdit struct {
	ID         string    `gorm:"primaryKey;type:text" json:"id"`
	AssetID    string    `gorm:"index;type:text" json:"assetId"`
	Action     string    `gorm:"type:text" json:"action"` // crop | rotate | mirror
	Parameters string    `gorm:"type:text" json:"parameters"`
	Sequence   int       `gorm:"type:int" json:"sequence"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Stack groups assets that share the same subject (manual stacking — no ML).
// The first asset id in the stack is the primary.
type Stack struct {
	ID             string    `gorm:"primaryKey;type:text" json:"id"`
	PrimaryAssetID string    `gorm:"index;type:text" json:"primaryAssetId"`
	OwnerID        string    `gorm:"index;type:text" json:"ownerId"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// StackAsset is the join table linking assets to a stack, ordered.
type StackAsset struct {
	StackID string `gorm:"primaryKey;type:text" json:"stackId"`
	AssetID string `gorm:"primaryKey;type:text" json:"assetId"`
	Order   int    `json:"order"`
}

// Memory is a user-curated memory collection (Immich's "Memories" feature).
// Only the curated memories are modeled here; the read-only "On this day"
// view is served separately from asset dates (see handleMemories).
type Memory struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	OwnerID   string    `gorm:"index;type:text" json:"ownerId"`
	Type      string    `gorm:"type:text" json:"type"` // on_this_day
	MemoryAt  time.Time `json:"memoryAt"`
	ShowAt    *time.Time `json:"showAt,omitempty"`
	HideAt    *time.Time `json:"hideAt,omitempty"`
	SeenAt    *time.Time `json:"seenAt,omitempty"`
	IsSaved   bool      `json:"isSaved"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Data carries the OnThisDayDto {year}.
	DataJSON  string `gorm:"type:text" json:"-"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// MemoryAsset is the join table linking assets to a memory, ordered.
type MemoryAsset struct {
	MemoryID string `gorm:"primaryKey;type:text" json:"memoryId"`
	AssetID  string `gorm:"primaryKey;type:text" json:"assetId"`
	Order    int    `json:"order"`
}

func (AssetEdit) TableName() string           { return "asset_edit" }
func (Stack) TableName() string               { return "stacks" }
func (StackAsset) TableName() string          { return "stacks_assets" }
func (Memory) TableName() string              { return "memories" }
func (MemoryAsset) TableName() string         { return "memories_assets" }
func (Person) TableName() string              { return "person" }
func (Activity) TableName() string            { return "activity" }
func (SharedLink) TableName() string          { return "shared_links" }
func (ApiKey) TableName() string              { return "api_keys" }
func (SystemConfig) TableName() string        { return "system_config" }
func (DuplicateResolution) TableName() string { return "duplicate_resolutions" }
func (SyncState) TableName() string           { return "sync_state" }
func (Session) TableName() string             { return "sessions" }
func (UserPreferences) TableName() string     { return "user_preferences" }
