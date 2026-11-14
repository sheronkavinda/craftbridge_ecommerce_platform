package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"craftbridge/backend/internal/modules/admin"
	"craftbridge/backend/internal/modules/audit"
	"craftbridge/backend/internal/modules/report"
	"craftbridge/backend/internal/platform/config"
	"craftbridge/backend/internal/platform/database"
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	   dbPool, err := database.NewPostgresPool(ctx, cfg.DBURL)
   if err != nil {
       log.Fatalf("❌ Failed to connect to database: %v", err)
   }
   defer dbPool.Close()
   log.Println("✅ Database connect successful")

	// Initialize Member 06 modules (Audit, Report, Admin)
	auditRepo := audit.NewRepository(dbPool)
	auditService := audit.NewService(auditRepo)
	auditHandler := audit.NewHandler(auditService)

	reportRepo := report.NewRepository(dbPool)
	reportService := report.NewService(reportRepo)
	reportHandler := report.NewHandler(reportService)

	adminRepo := admin.NewRepository(dbPool)
	adminService := admin.NewService(adminRepo, auditService)
	adminHandler := admin.NewHandler(adminService)

	router := platformhttp.NewRouter(
		cfg,
		dbPool,
		func(v1 *gin.RouterGroup) {
			audit.RegisterRoutes(v1, auditHandler)
			report.RegisterRoutes(v1, reportHandler)
			admin.RegisterRoutes(v1, adminHandler)
		},
	)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("CraftBridge API server starting on port %s (env: %s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	log.Println("shutting down CraftBridge API server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server exited cleanly")
}
