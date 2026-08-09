package app

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store wraps the database handle and bootstrap logic.
type Store struct {
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

	models := []interface{}{
		&User{}, &Asset{}, &Exif{}, &Album{}, &AlbumAsset{},
		&Library{}, &Partner{}, &Tag{}, &AssetTag{}, &Person{},
		&Activity{}, &SharedLink{}, &ApiKey{}, &SystemConfig{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		return nil, err
	}

	s := &Store{DB: db}
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
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
			ID:                    newUUID(),
			Email:                 adminEmail,
			Name:                  "Administrator",
			Password:              string(hash),
			Salt:                  newUUID(),
			IsAdmin:               true,
			ShouldChangePassword: false,
			AvatarColor:          "primary",
			CreatedAt:             time.Now().UTC(),
			UpdatedAt:             time.Now().UTC(),
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
