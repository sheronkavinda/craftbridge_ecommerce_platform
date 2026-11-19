package m04

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"craftbridge/backend/internal/modules/cart"
	"craftbridge/backend/internal/modules/order"
	"craftbridge/backend/internal/modules/shipping"
)

// Register initializes all Shalinda backend modules.
func Register(
	ctx context.Context,
	api *gin.RouterGroup,
	db *pgxpool.Pool,
	authenticate gin.HandlerFunc,
	inventory order.InventoryManager,
) (*order.Service, error) {

	if ctx == nil ||
		api == nil ||
		db == nil ||
		authenticate == nil ||
		inventory == nil {
		return nil, errors.New(
			"missing Member 04 integration dependency",
		)
	}

	// ===================================
	// 1. Cart Module
	// ===================================

	cartRepository := cart.NewRepository(db)
	cartHandler := cart.NewHandler(cartRepository)

	cart.RegisterRoutes(
		api,
		cartHandler,
		authenticate,
	)

	// ===================================
	// 2. Checkout and Orders Module
	// ===================================

	orderService := order.NewService(
		db,
		inventory,
	)

	orderHandler := order.NewHandler(
		orderService,
	)

	order.RegisterRoutes(
		api,
		orderHandler,
		authenticate,
	)

	// ===================================
	// 3. Shipping and Delivery Module
	// ===================================

	shippingService := shipping.NewService(db)

	shippingHandler := shipping.NewHandler(
		shippingService,
	)

	shipping.RegisterRoutes(
		api,
		shippingHandler,
		authenticate,
	)

	// ===================================
	// 4. Seller Shipping Rates
	// ===================================

	shipping.RegisterShippingRateRoutes(
		api,
		shippingHandler,
		authenticate,
	)

	// ===================================
	// 5. Automated Order Expiry
	// ===================================

	if os.Getenv("ORDER_EXPIRY_ENABLED") == "1" {
		order.StartExpiryWorker(
			ctx,
			orderService,
			time.Minute,
			50,
		)
	}

	return orderService, nil
}
