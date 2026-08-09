package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const ctxUserID = "userID"

// Claims is the JWT payload (mirrors Immich's auth token shape loosely).
type Claims struct {
	UserID string `json:"userID"`
	jwt.RegisteredClaims
}

func (a *App) issueToken(userID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "immich-go",
			Audience:  jwt.ClaimStrings{"immich-go"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(60 * 24 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(a.cfg.JWTSecret))
}

func (a *App) parseToken(t string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(t, claims, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// AuthGuard authenticates via Bearer JWT or an Immich-Api-Key / X-Api-Key header.
func (a *App) AuthGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		var uid string
		if key := c.GetHeader("x-api-key"); key != "" {
			var ak ApiKey
			if err := a.store.DB.Where("key = ?", hashKey(a.cfg.APIKeySalt, key)).First(&ak).Error; err == nil {
				uid = ak.UserID
			}
		}
		if uid == "" {
			h := c.GetHeader("Authorization")
			if strings.HasPrefix(h, "Bearer ") {
				if claims, err := a.parseToken(strings.TrimPrefix(h, "Bearer ")); err == nil {
					uid = claims.UserID
				}
			}
		}
		if uid == "" {
			if !a.cfg.LoginRequired {
				// anonymous mode: fall back to the admin user
				var admin User
				if err := a.store.DB.Where("is_admin = ?", true).First(&admin).Error; err == nil {
					uid = admin.ID
				}
			}
		}
		if uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "statusCode": 401})
			return
		}
		c.Set(ctxUserID, uid)
		c.Next()
	}
}

func currentUserID(c *gin.Context) string {
	if v, ok := c.Get(ctxUserID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func hashKey(salt, key string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(salt+key), bcrypt.MinCost)
	return string(h)
}

// ---- handlers ----

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "statusCode": 400})
		return
	}
	var u User
	if err := a.store.DB.Where("email = ?", req.Email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials", "statusCode": 401})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials", "statusCode": 401})
		return
	}
	token, err := a.issueToken(u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"accessToken":         token,
		"userToken":           token,
		"userId":              u.ID,
		"userEmail":           u.Email,
		"name":                u.Name,
		"isAdmin":             u.IsAdmin,
		"shouldChangePassword": u.ShouldChangePassword,
	})
}

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (a *App) handleSignup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "statusCode": 400})
		return
	}
	var n int64
	a.store.DB.Model(&User{}).Count(&n)
	if n > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration disabled; an account already exists", "statusCode": 403})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u := User{
		ID:        newUUID(),
		Email:     req.Email,
		Name:      req.Name,
		Password:  string(hash),
		Salt:      newUUID(),
		IsAdmin:   true,
		AvatarColor: "primary",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := a.store.DB.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	token, _ := a.issueToken(u.ID)
	c.JSON(http.StatusCreated, gin.H{"accessToken": token, "userId": u.ID, "userEmail": u.Email, "name": u.Name, "isAdmin": true})
}

func (a *App) handleValidate(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "statusCode": 401})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"accessToken":  "",
		"userId":       u.ID,
		"userEmail":    u.Email,
		"name":         u.Name,
		"isAdmin":      u.IsAdmin,
		"shouldChangePassword": u.ShouldChangePassword,
	})
}

type changePassRequest struct {
	Password     string `json:"password"`
	NewPassword  string `json:"newPassword"`
}

func (a *App) handleChangePassword(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req changePassRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "statusCode": 400})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wrong password", "statusCode": 400})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	u.Password = string(hash)
	u.ShouldChangePassword = false
	a.store.DB.Save(&u)
	c.Status(http.StatusNoContent)
}

func (a *App) handleLogout(c *gin.Context) {
	// Stateless JWT: logout is a client-side no-op. Kept for API compatibility.
	c.Status(http.StatusOK)
}

func (a *App) handleApiKeys(c *gin.Context) {
	uid := currentUserID(c)
	switch c.Request.Method {
	case http.MethodGet:
		var keys []ApiKey
		a.store.DB.Where("user_id = ?", uid).Find(&keys)
		c.JSON(http.StatusOK, keys)
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		_ = c.ShouldBindJSON(&body)
		raw := newUUID() + newUUID()
		ak := ApiKey{
			ID:        newUUID(),
			UserID:    uid,
			Name:      body.Name,
			Key:       hashKey(a.cfg.APIKeySalt, raw),
			CreatedAt: time.Now().UTC(),
		}
		a.store.DB.Create(&ak)
		c.JSON(http.StatusCreated, gin.H{"id": ak.ID, "name": ak.Name, "key": raw, "createdAt": ak.CreatedAt})
	case http.MethodDelete:
		id := c.Param("id")
		a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&ApiKey{})
		c.Status(http.StatusOK)
	}
}
