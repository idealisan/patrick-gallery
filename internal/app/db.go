package app

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"immich-go/internal/store"
)

// Store wraps the database handle and bootstrap logic.
//
// It embeds the store.Store abstraction (see internal/store) and shadows its
// DB() method with a concrete *gorm.DB field so existing handler code that
// does `a.store.DB.Where(...)` keeps working unchanged. The embedded
// interface is the seam for a future Postgres backend.
type Store struct {
	store.Store
	DB *gorm.DB
}

// OpenDB opens (and auto-migrates) the SQLite database and seeds an initial
// admin user + system config if the database is empty.
func OpenDB(dbPath, resourceDir string) (*Store, error) {
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		return nil, err
	}
	for _, d := range []string{"upload", "thumbnail", "encoded-video", "profile", "library"} {
		_ = os.MkdirAll(filepath.Join(resourceDir, d), 0o755)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	// Single-user / private-LAN tuning: WAL for concurrent readers + a writer,
	// a generous busy_timeout so short write contention waits instead of
	// erroring, and NORMAL synchronous for speed without sacrificing durability
	// across app crashes (still safe vs power loss at WAL default).
	for _, p := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA cache_size=-8000;", // ~8 MB page cache
	} {
		db.Exec(p)
	}

	models := []interface{}{
		&User{}, &Asset{}, &Exif{}, &Album{}, &AlbumAsset{}, &AlbumUser{},
		&Library{}, &Partner{}, &Tag{}, &AssetTag{}, &Person{},
		&Activity{}, &SharedLink{}, &ApiKey{}, &SystemConfig{},
		&DuplicateResolution{}, &SyncState{}, &Session{}, &UserPreferences{},
		&NotificationToken{}, &Notification{}, &AssetEdit{}, &Stack{}, &StackAsset{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		return nil, err
	}

	s := &Store{Store: store.NewSQLite(db), DB: db}
	if err := s.seed(); err != nil {
		return nil, err
	}
	// Backfill user.status for rows created before the column existed so the
	// official v3.1.0 mobile client's UserAdminResponseDto (which requires a
	// valid, non-null status) never receives an empty/unknown value.
	if err := s.DB.Model(&User{}).Where("status = '' OR status IS NULL").Update("status", "active").Error; err != nil {
		return nil, err
	}
	if err := s.ensureJWTSecret(); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureJWTSecret guarantees the singleton SystemConfig row carries a stable,
// per-instance HMAC key. The first run generates one and persists it; later
// runs reuse it so issued tokens survive restarts. Because the key is unique
// per deployment (not the old compile-time constant), any token a browser
// cached from a previous instance is rejected, forcing the login screen.
func (s *Store) ensureJWTSecret() error {
	var cfg SystemConfig
	if err := s.DB.First(&cfg, "id = ?", "singleton").Error; err != nil {
		return err
	}
	if cfg.JWTSecret != "" {
		return nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	cfg.JWTSecret = hex.EncodeToString(buf)
	return s.DB.Save(&cfg).Error
}

func (s *Store) seed() error {
	var n int64
	s.DB.Model(&User{}).Count(&n)
	if n == 0 {
		adminEmail := getEnv("IMMICH_ADMIN_EMAIL", "admin@immich.app")
		adminPass := getEnv("IMMICH_ADMIN_PASSWORD", "password")
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		admin := &User{
			ID:                   newUUID(),
			Email:                adminEmail,
			Name:                 "Administrator",
			Password:             string(hash),
			Salt:                 newUUID(),
			IsAdmin:              true,
			ShouldChangePassword: false,
			AvatarColor:          "primary",
			Status:               "active",
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}
		if err := s.DB.Create(admin).Error; err != nil {
			return err
		}
		log.Printf("[seed] created default admin user %s (password: %s)", adminEmail, adminPass)
	}

	var cn int64
	s.DB.Model(&SystemConfig{}).Count(&cn)
	if cn == 0 {
		cfg := &SystemConfig{
			ID:            "singleton",
			LoginRequired: true,
			IsPublic:      false,
		}
		if err := s.DB.Create(cfg).Error; err != nil {
			return err
		}
	}
	// ensure a default upload library exists per admin
	var libN int64
	s.DB.Model(&Library{}).Count(&libN)
	if libN == 0 {
		var admin User
		s.DB.First(&admin)
		lib := &Library{
			ID:        newUUID(),
			OwnerID:   admin.ID,
			Name:      "Default Upload Library",
			Type:      "UPLOAD",
			Watched:   false,
			Status:    "inactive",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		_ = s.DB.Create(lib).Error
	}
	return nil
}
