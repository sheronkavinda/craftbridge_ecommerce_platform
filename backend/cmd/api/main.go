package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"craftbridge/backend/internal/modules/account"
	"craftbridge/backend/internal/modules/admin"
	"craftbridge/backend/internal/modules/audit"
	"craftbridge/backend/internal/modules/auth"
	"craftbridge/backend/internal/modules/catalog"
	"craftbridge/backend/internal/modules/m04"
	"craftbridge/backend/internal/modules/payment"
	"craftbridge/backend/internal/modules/receipt"
	"craftbridge/backend/internal/modules/refund"
	"craftbridge/backend/internal/modules/report"
	"craftbridge/backend/internal/modules/seller"
	"craftbridge/backend/internal/modules/storefront"
	"craftbridge/backend/internal/modules/wishlist"
	"craftbridge/backend/internal/platform/config"
	"craftbridge/backend/internal/platform/database"
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

type paymentAuditAdapter struct {
	auditSvc *audit.Service
}

func (a *paymentAuditAdapter) RecordAdminAction(
	ctx context.Context,
	actor payment.Actor,
	requestID, action, target, outcome, details string,
) {
	if a == nil || a.auditSvc == nil {
		return
	}
	_, _ = a.auditSvc.LogAction(ctx, audit.CreateAuditLogInput{
		ActorID:   actor.ID,
		ActorName: actor.Name,
		Action:    action,
		Target:    target,
		Outcome:   outcome,
		RequestID: requestID,
		Details:   details,
	})
}

type refundAuditAdapter struct {
	auditSvc *audit.Service
}

func (a *refundAuditAdapter) RecordAdminAction(
	ctx context.Context,
	actor refund.Actor,
	requestID, action, target, outcome, details string,
) {
	if a == nil || a.auditSvc == nil {
		return
	}
	_, _ = a.auditSvc.LogAction(ctx, audit.CreateAuditLogInput{
		ActorID:   actor.ID,
		ActorName: actor.Name,
		Action:    action,
		Target:    target,
		Outcome:   outcome,
		RequestID: requestID,
		Details:   details,
	})
}

type refundGatewayAdapter struct {
	refunder payment.Refunder
}

func (g *refundGatewayAdapter) RefundPayment(
	ctx context.Context,
	input refund.GatewayRefundInput,
) (refund.GatewayRefundOutput, error) {
	if g == nil || g.refunder == nil {
		return refund.GatewayRefundOutput{}, refund.ErrGatewayNotConfigured
	}
	res, err := g.refunder.RefundPayment(ctx, payment.RefundRequestInput{
		ProviderReference: input.ProviderReference,
		AmountCents:       input.AmountCents,
		Currency:          input.Currency,
		Reason:            input.Reason,
		IdempotencyKey:    input.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, payment.ErrProviderNotConfigured) {
			return refund.GatewayRefundOutput{}, refund.ErrGatewayNotConfigured
		}
		return refund.GatewayRefundOutput{}, err
	}

	return refund.GatewayRefundOutput{
		ProviderRefundReference: res.ProviderRefundReference,
		Status:                  refund.GatewayRefundOutcome(res.Status),
		Raw:                     res.Raw,
	}, nil
}

type refundPaymentReaderAdapter struct {
	paymentRepo *payment.Repository
}

func (p *refundPaymentReaderAdapter) GetPaidAttemptForOrder(
	ctx context.Context,
	orderID string,
) (refund.PaidPayment, error) {
	if p == nil || p.paymentRepo == nil {
		return refund.PaidPayment{}, refund.ErrNoVerifiedPayment
	}

	ord, err := p.paymentRepo.GetOrderForPayment(ctx, orderID)
	if err != nil {
		if errors.Is(err, payment.ErrOrderNotFound) {
			return refund.PaidPayment{}, refund.ErrOrderNotFound
		}
		return refund.PaidPayment{}, err
	}

	att, err := p.paymentRepo.GetLatestSuccessfulAttemptByOrder(ctx, ord.OrderID)
	if err != nil {
		return refund.PaidPayment{}, err
	}
	if att == nil {
		return refund.PaidPayment{
			OrderID:     ord.OrderID,
			OrderNumber: ord.OrderNumber,
			BuyerID:     ord.UserID,
			SellerID:    ord.SellerID,
		}, refund.ErrNoVerifiedPayment
	}

	provRef := att.ID
	if att.ProviderReference != nil && strings.TrimSpace(*att.ProviderReference) != "" {
		provRef = strings.TrimSpace(*att.ProviderReference)
	}
	completedAt := att.UpdatedAt
	if att.CompletedAt != nil {
		completedAt = *att.CompletedAt
	}

	return refund.PaidPayment{
		AttemptID:         att.ID,
		OrderID:           ord.OrderID,
		OrderNumber:       ord.OrderNumber,
		BuyerID:           ord.UserID,
		SellerID:          ord.SellerID,
		ProviderReference: provRef,
		AmountCents:       att.AmountCents,
		Currency:          att.Currency,
		CompletedAt:       completedAt,
	}, nil
}

type receiptPaymentReaderAdapter struct {
	paymentRepo *payment.Repository
}

func (r *receiptPaymentReaderAdapter) GetVerifiedPaymentForOrder(
	ctx context.Context,
	orderID string,
) (receipt.PaidOrderPayment, error) {
	if r == nil || r.paymentRepo == nil {
		return receipt.PaidOrderPayment{}, receipt.ErrNoVerifiedPayment
	}

	ord, err := r.paymentRepo.GetOrderForPayment(ctx, orderID)
	if err != nil {
		if errors.Is(err, payment.ErrOrderNotFound) {
			return receipt.PaidOrderPayment{}, receipt.ErrOrderNotFound
		}
		return receipt.PaidOrderPayment{}, err
	}

	att, err := r.paymentRepo.GetLatestSuccessfulAttemptByOrder(ctx, ord.OrderID)
	if err != nil {
		return receipt.PaidOrderPayment{}, err
	}
	if att == nil {
		return receipt.PaidOrderPayment{
			OrderID:     ord.OrderID,
			OrderNumber: ord.OrderNumber,
			BuyerID:     ord.UserID,
			BuyerName:   ord.BuyerName,
			BuyerEmail:  ord.BuyerEmail,
		}, receipt.ErrNoVerifiedPayment
	}

	provRef := att.ID
	if att.ProviderReference != nil && strings.TrimSpace(*att.ProviderReference) != "" {
		provRef = strings.TrimSpace(*att.ProviderReference)
	}
	completedAt := att.UpdatedAt
	if att.CompletedAt != nil {
		completedAt = *att.CompletedAt
	}

	return receipt.PaidOrderPayment{
		AttemptID:         att.ID,
		OrderID:           ord.OrderID,
		OrderNumber:       ord.OrderNumber,
		BuyerID:           ord.UserID,
		BuyerName:         ord.BuyerName,
		BuyerEmail:        ord.BuyerEmail,
		AmountCents:       att.AmountCents,
		Currency:          att.Currency,
		Provider:          att.Provider,
		ProviderReference: provRef,
		CompletedAt:       completedAt,
	}, nil
}

type receiptOrderSummaryAdapter struct{}

func (receiptOrderSummaryAdapter) GetOrderSummary(_ context.Context, _ string) (receipt.OrderSummary, error) {
	// With the current placeholder orders schema (which has no order_items table yet),
	// return an empty items slice without inventing line items.
	return receipt.OrderSummary{
		Items: []receipt.ReceiptLineItem{},
	}, nil
}

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

	// Initialize Member 02 modules (Seller, Catalog, Inventory, Media).
	sellerRepo := seller.NewRepository(dbPool)
	sellerHandler := seller.NewHandler(seller.NewService(sellerRepo))

	catalogRepo := catalog.NewRepository(dbPool)
	catalogService := catalog.NewService(catalogRepo)
	catalogHandler := catalog.NewHandler(catalogService)

	catalogUploadDir := cfg.CatalogUploadDir
	imageStore, err := catalog.NewLocalImageStore(catalogUploadDir)
	if err != nil {
		log.Fatalf("initialize catalog image storage: %v", err)
	}
	shopMediaHandler := seller.NewShopMediaHandler(sellerRepo, imageStore)
	imageHandler := catalog.NewImageHandler(catalogRepo, imageStore)

	paymentProvider := payment.NewPayHereSandboxProvider(cfg)
	paymentRepo := payment.NewRepository(dbPool)
	paymentService := payment.NewService(paymentRepo, paymentRepo, paymentProvider)
	paymentService.SetReconcileConfig(
		time.Duration(cfg.PaymentReconcileAfterMinutes)*time.Minute,
		time.Duration(cfg.PaymentAttemptTTLHours)*time.Hour,
	)
	paymentService.SetEarningsConfig(cfg.PlatformCommissionPercent, cfg.PayoutHoldDays)
	paymentService.SetAuditRecorder(&paymentAuditAdapter{auditSvc: auditService})
	paymentService.StartReconcileWorker(ctx, time.Duration(cfg.PaymentReconcileIntervalSeconds)*time.Second)
	paymentHandler := payment.NewHandler(paymentService)

	refundRepo := refund.NewRepository(dbPool)
	refundService := refund.NewService(
		refundRepo,
		&refundPaymentReaderAdapter{paymentRepo: paymentRepo},
		&refundGatewayAdapter{refunder: paymentProvider},
	)
	refundService.SetRefundWindowDays(cfg.RefundWindowDays)
	refundService.SetAuditRecorder(&refundAuditAdapter{auditSvc: auditService})
	refundHandler := refund.NewHandler(refundService)

	receiptRepo := receipt.NewRepository(dbPool)
	receiptService := receipt.NewService(
		receiptRepo,
		&receiptPaymentReaderAdapter{paymentRepo: paymentRepo},
		receiptOrderSummaryAdapter{},
	)
	receiptHandler := receipt.NewHandler(receiptService)

	authRepo := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepo, dbPool, cfg)
	authService.SetPasswordResetEmailSender(auth.NewSMTPPasswordResetSender(cfg))
	authHandler := auth.NewHandler(authService)

	accountRepo := account.NewRepository(dbPool)
	accountService := account.NewService(accountRepo)
	accountHandler := account.NewHandler(accountService)

	router := platformhttp.NewRouter(
		cfg,
		dbPool,
		func(v1 *gin.RouterGroup) {
			auth.RegisterRoutes(v1, authHandler)
			account.RegisterRoutes(v1, accountHandler)
			audit.RegisterRoutes(v1, auditHandler)
			report.RegisterRoutes(v1, reportHandler)
			admin.RegisterRoutes(v1, adminHandler)

			authenticate := platformhttp.RequireAuthenticatedUser()
			if _, err := m04.Register(ctx, v1, dbPool, authenticate, catalogService); err != nil {
				log.Fatalf("register cart, checkout and shipping routes: %v", err)
			}

			seller.RegisterRoutes(v1, sellerHandler, authenticate)
			seller.RegisterShopMediaRoutes(v1, shopMediaHandler, authenticate)
			seller.RegisterAdminRoutes(v1, sellerHandler, authenticate, platformhttp.RequireAdminRole())
			catalog.RegisterRoutes(v1, catalogHandler, authenticate)
			catalog.RegisterInventoryRoutes(v1, catalogHandler, authenticate)
			catalog.RegisterImageRoutes(v1, imageHandler, authenticate)

			storefront.RegisterRoutes(v1)
			wishlist.RegisterRoutes(v1, dbPool)
			payment.RegisterRoutes(v1, paymentHandler)
			refund.RegisterRoutes(v1, refundHandler)
			receipt.RegisterRoutes(v1, receiptHandler)
		},
	)

	router.StaticFS("/media/catalog", gin.Dir(catalogUploadDir, false))

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
			log.Fatalf("server forced to stop: %v", err)
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
