package app

import (
	"crypto/sha256"
	"encoding/hex"
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

// Cookie names mirror the official Immich server (server/src/enum.ts ->
// ImmichCookie). The official v3.1.0 web reads `immich_is_authenticated`
// (non-httpOnly, so JS can see auth state) to decide whether to call the API,
// and sends `immich_access_token` (httpOnly) on every same-origin request.
const (
	cookieAccessToken  = "immich_access_token"
	cookieIsAuth       = "immich_is_authenticated"
	authCookieMaxAge   = 400 * 24 * 3600 // seconds, matches official (400 days)
)

// isSecureRequest reports whether the connection to the *client* is TLS,
// accounting for a TLS-terminating reverse proxy that forwards the original
// scheme via X-Forwarded-Proto. This is required so the Secure cookie
// attribute is set correctly when deployed behind a reverse proxy.
func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	return proto == "https" || proto == "HTTPS"
}

// setAuthCookies replicates the official server's login cookie behaviour so
// the official web UI authenticates correctly. See server/src/utils/response.ts
// respondWithCookie for the attribute source of truth.
func (a *App) setAuthCookies(c *gin.Context, token string) {
	secure := isSecureRequest(c)
	// immich_access_token: httpOnly session token.
	c.SetCookie(cookieAccessToken, token, authCookieMaxAge, "/", "", secure, true)
	// immich_is_authenticated: readable by JS so the web knows auth state.
	c.SetCookie(cookieIsAuth, "true", authCookieMaxAge, "/", "", secure, false)
}

func (a *App) clearAuthCookies(c *gin.Context) {
	secure := isSecureRequest(c)
	c.SetCookie(cookieAccessToken, "", -1, "/", "", secure, true)
	c.SetCookie(cookieIsAuth, "", -1, "/", "", secure, false)
}

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
	return tok.SignedString([]byte(a.jwtSecret))
}

func (a *App) parseToken(t string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(t, claims, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(a.jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// AuthGuard authenticates the request. immich-go issues stateless JWT access
// tokens (the official server uses DB-backed sessions, but the *client
// contract* is identical: a bearer token string). We accept that token from
// every source the official Immich web & mobile clients actually use, in the
// same precedence the official server uses (server/src/services/auth.service.ts
// validate()): x-immich-user-token / x-immich-session-token / Authorization:
// Bearer / immich_access_token cookie, then x-api-key (real API keys are
// bcrypt-hashed in the api_keys table; a JWT sent via x-api-key is also
// accepted for compatibility with SDK clients).
func (a *App) AuthGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		var uid string
		var token string

		if token == "" {
			token = c.GetHeader("x-immich-user-token")
		}
		if token == "" {
			token = c.GetHeader("x-immich-session-token")
		}
		if token == "" {
			if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if token == "" {
			if ck, err := c.Cookie(cookieAccessToken); err == nil {
				token = ck
			}
		}
		if token == "" {
			token = c.GetHeader("x-api-key")
		}

		if token != "" {
			if claims, err := a.parseToken(token); err == nil {
				uid = claims.UserID
			} else if key := c.GetHeader("x-api-key"); key != "" {
				// Real API key (bcrypt lookup) sent via x-api-key.
				var ak ApiKey
				if err := a.store.DB.Where("key = ?", hashKey(a.cfg.APIKeySalt, key)).First(&ak).Error; err == nil {
					uid = ak.UserID
				}
			}
		}

		if uid == "" && !a.cfg.LoginRequired {
			// anonymous mode: fall back to the admin user
			var admin User
			if err := a.store.DB.Where("is_admin = ?", true).First(&admin).Error; err == nil {
				uid = admin.ID
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

// requestToken extracts the raw auth token from the request using the same
// precedence as AuthGuard. It is needed by the session endpoints so a JWT can
// be linked back to its Session row (via the hashed token).
func requestToken(c *gin.Context) string {
	if t := c.GetHeader("x-immich-user-token"); t != "" {
		return t
	}
	if t := c.GetHeader("x-immich-session-token"); t != "" {
		return t
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if ck, err := c.Cookie(cookieAccessToken); err == nil {
		return ck
	}
	return c.GetHeader("x-api-key")
}

func hashKey(salt, key string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(salt+key), bcrypt.MinCost)
	return string(h)
}

// hashToken returns a stable, non-reversible hash of an auth token so a JWT
// can be linked back to its Session row (for session-lock state) without
// storing the raw bearer credential.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
	a.recordSession(u.ID, token)
	a.setAuthCookies(c, token)
	c.JSON(http.StatusCreated, gin.H{
		"accessToken":          token,
		"userToken":            token,
		"userId":               u.ID,
		"userEmail":            u.Email,
		"name":                 u.Name,
		"isAdmin":              u.IsAdmin,
		"shouldChangePassword": u.ShouldChangePassword,
		// Required by the official v3.1.0 LoginResponseDto contract. Without
		// these the openapi-generated iOS/Android client throws on
		// deserialization and reports "login failed".
		"isOnboarded":      true,
		"profileImagePath": u.ProfileImagePath,
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
		ID:          newUUID(),
		Email:       req.Email,
		Name:        req.Name,
		Password:    string(hash),
		Salt:        newUUID(),
		IsAdmin:     true,
		AvatarColor: "primary",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := a.store.DB.Create(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	token, _ := a.issueToken(u.ID)
	a.recordSession(u.ID, token)
	a.setAuthCookies(c, token)
	c.JSON(http.StatusCreated, gin.H{"accessToken": token, "userId": u.ID, "userEmail": u.Email, "name": u.Name, "isAdmin": true, "isOnboarded": true, "profileImagePath": u.ProfileImagePath, "shouldChangePassword": false})
}

func (a *App) handleValidate(c *gin.Context) {
	uid := currentUserID(c)
	var u User
	if err := a.store.DB.First(&u, "id = ?", uid).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "statusCode": 401})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"accessToken":          "",
		"userId":               u.ID,
		"userEmail":            u.Email,
		"name":                 u.Name,
		"isAdmin":              u.IsAdmin,
		"shouldChangePassword": u.ShouldChangePassword,
		"isOnboarded":          true,
		"profileImagePath":     u.ProfileImagePath,
	})
}

type changePassRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"newPassword"`
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
	// Mirror the official server's logout contract (server/src/api/auth/
	// auth.controller.ts -> logout): delete the server-side session and clear
	// the auth cookies, then return the LogoutResponseDto the official web &
	// mobile clients parse. Returning an empty body here crashes the Flutter
	// SDK with "FormatException: Unexpected character" (see logs/), so the
	// JSON body is mandatory contract, not optional.
	if tok := requestToken(c); tok != "" {
		if a.store != nil {
			a.store.DB.Where("token = ?", hashToken(tok)).Delete(&Session{})
		}
	}
	a.clearAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{
		"successful":  true,
		"redirectUri": "/auth/login?autoLaunch=0",
	})
}

func (a *App) handleApiKeys(c *gin.Context) {
	uid := currentUserID(c)
	switch c.Request.Method {
	case http.MethodGet:
		if id := c.Param("id"); id != "" {
			var ak ApiKey
			if err := a.store.DB.First(&ak, "id = ? AND user_id = ?", id, uid).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(http.StatusOK, ak)
			return
		}
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
	case http.MethodPut:
		id := c.Param("id")
		var ak ApiKey
		if err := a.store.DB.First(&ak, "id = ? AND user_id = ?", id, uid).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Name != "" {
			ak.Name = body.Name
		}
		a.store.DB.Save(&ak)
		c.JSON(http.StatusOK, ak)
	case http.MethodDelete:
		id := c.Param("id")
		a.store.DB.Where("id = ? AND user_id = ?", id, uid).Delete(&ApiKey{})
		c.Status(http.StatusOK)
	}
}
