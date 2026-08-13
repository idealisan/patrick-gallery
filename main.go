package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"immich-go/internal/app"
	"immich-go/internal/webroot"
)

func main() {
	cfg := app.LoadConfig()
	store, err := app.OpenDB(cfg.DBPath, cfg.ResourceDir)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	application := app.NewApp(cfg, store)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	application.RegisterRoutes(r)

	// Serve the embedded SPA for every non-API route (SPA fallback).
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found", "statusCode": 404})
			return
		}
		webroot.Serve(c.Writer, c.Request)
	})

	addr := cfg.Host + ":" + strconv.Itoa(cfg.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("[immich-go] listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("[immich-go] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("[immich-go] bye")
}
