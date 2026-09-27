package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"facility-service/internal/config"
	"facility-service/internal/handler"
	"facility-service/internal/middleware"
	"facility-service/internal/model"
	"facility-service/internal/repository"
	"facility-service/internal/router"
	"facility-service/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db := connectDB(cfg.DatabaseURL)
	if err := db.AutoMigrate(&model.Facility{}, &model.FacilityAvailability{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	auth := connectKeycloak(ctx, cfg)

	repo := repository.NewFacilityRepository(db)
	svc := service.NewFacilityService(repo)
	h := handler.NewFacilityHandler(svc)
	r := router.New(h, auth)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("facility-service listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("facility-service stopped")
}

// Postgres ใน docker-compose อาจยังไม่พร้อมตอน service start → retry
func connectDB(dsn string) *gorm.DB {
	var lastErr error
	for i := 0; i < 15; i++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			if sqlDB, e := db.DB(); e == nil && sqlDB.Ping() == nil {
				return db
			}
		}
		lastErr = err
		log.Printf("waiting for database... (%d/15)", i+1)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("database: %v", lastErr)
	return nil
}

// Keycloak มัก start ช้ากว่า service → retry
func connectKeycloak(ctx context.Context, cfg config.Config) gin.HandlerFunc {
	var lastErr error
	for i := 0; i < 30; i++ {
		auth, err := middleware.NewKeycloakAuth(ctx, cfg.JWKSURL(), cfg.KeycloakIssuer)
		if err == nil {
			return auth
		}
		lastErr = err
		log.Printf("waiting for keycloak... (%d/30)", i+1)
		time.Sleep(3 * time.Second)
	}
	log.Fatalf("keycloak: %v", lastErr)
	return nil
}
