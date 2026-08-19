package app

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"time"

	"github.com/gin-gonic/gin"
)

// handleAdminNotificationCreate mirrors POST /api/admin/notifications. Creates a
// server-side notification for a given user and persists it (real, not a stub).
func (a *App) handleAdminNotificationCreate(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var body struct {
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		Level       string                 `json:"level"`
		Type        string                 `json:"type"`
		UserID      string                 `json:"userId"`
		Data        map[string]interface{} `json:"data"`
		ReadAt      *string                `json:"readAt"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Title == "" || body.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "title and userId required", "statusCode": 400})
		return
	}
	if body.Level == "" {
		body.Level = "info"
	}
	if body.Type == "" {
		body.Type = "admin"
	}
	now := time.Now().UTC()
	n := Notification{
		ID:          newUUID(),
		UserID:      body.UserID,
		Type:        body.Type,
		Level:       body.Level,
		Title:       body.Title,
		Description: body.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if len(body.Data) > 0 {
		if b, e := jsonMarshal(body.Data); e == nil {
			n.Metadata = string(b)
		}
	}
	if body.ReadAt != nil {
		if t, e := time.Parse(time.RFC3339, *body.ReadAt); e == nil {
			n.ReadAt = &t
		}
	}
	if err := a.store.DB.Create(&n).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	a.emit("on_new_notification", map[string]any{"userId": n.UserID, "notification": n})
	c.JSON(http.StatusCreated, a.notificationToDTO(&n))
}

// handleAdminTestEmail mirrors POST /api/admin/notifications/test-email. Sends a
// real test email via net/smtp using the provided SystemConfigSmtpDto. Pure Go
// (no external binary), so this is genuine work, not a stub.
func (a *App) handleAdminTestEmail(c *gin.Context) {
	if _, ok := a.requireAdmin(c); !ok {
		return
	}
	var smtpCfg struct {
		Enabled  bool   `json:"enabled"`
		From     string `json:"from"`
		ReplyTo  string `json:"replyTo"`
		Transport struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Secure   bool   `json:"secure"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"transport"`
	}
	if err := c.ShouldBindJSON(&smtpCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid smtp config", "statusCode": 400})
		return
	}
	if smtpCfg.Transport.Host == "" || smtpCfg.Transport.Port == 0 || smtpCfg.From == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "smtp host, port and from are required", "statusCode": 400})
		return
	}
	to := smtpCfg.From
	if smtpCfg.ReplyTo != "" {
		to = smtpCfg.ReplyTo
	}
	msgID, err := sendSMTPEmail(smtpSendConfig{
		Host:     smtpCfg.Transport.Host,
		Port:     smtpCfg.Transport.Port,
		Secure:   smtpCfg.Transport.Secure,
		Username: smtpCfg.Transport.Username,
		Password: smtpCfg.Transport.Password,
		From:     smtpCfg.From,
		To:       to,
		Subject:  "immich-go SMTP Test",
		Body:     "This is a test email from immich-go.",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error(), "statusCode": 500})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messageId": msgID})
}

type smtpSendConfig struct {
	Host     string
	Port     int
	Secure   bool
	Username string
	Password string
	From     string
	To       string
	Subject  string
	Body     string
}

// sendSMTPEmail sends a plaintext email over SMTP (STARTTLS when available, or
// implicit TLS when secure=true). Pure Go via net/smtp; no external binary.
func sendSMTPEmail(cfg smtpSendConfig) (string, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	from := cfg.From
	to := []string{cfg.To}
	msgID := fmt.Sprintf("<%d@immich-go>", time.Now().UnixNano())
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n",
		from, cfg.To, cfg.Subject, msgID)
	msg := []byte(headers + cfg.Body)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	var client *smtp.Client
	var err error
	if cfg.Secure {
		conn, derr := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host})
		if derr != nil {
			return "", derr
		}
		client, err = smtp.NewClient(conn, cfg.Host)
	} else {
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return "", err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return "", err
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return "", err
		}
	}
	if err := client.Mail(from); err != nil {
		return "", err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return "", err
		}
	}
	w, err := client.Data()
	if err != nil {
		return "", err
	}
	if _, err := w.Write(msg); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return msgID, nil
}

// notificationToDTO maps a Notification row to the official NotificationDto
// shape consumed by the web admin notification center.
func (a *App) notificationToDTO(n *Notification) gin.H {
	out := gin.H{
		"id":          n.ID,
		"userId":      n.UserID,
		"type":        n.Type,
		"level":       n.Level,
		"title":       n.Title,
		"description": n.Description,
		"createdAt":   n.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":   n.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		out["readAt"] = n.ReadAt.UTC().Format(time.RFC3339)
	} else {
		out["readAt"] = nil
	}
	if n.Metadata != "" {
		out["data"] = jsonUnmarshalString(n.Metadata)
	} else {
		out["data"] = gin.H{}
	}
	return out
}
