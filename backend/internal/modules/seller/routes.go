
package seller

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("seller routes require authentication")
	}

	api.GET("/shops/:slug", handler.GetPublicShop)
	sellerRoutes := api.Group("/seller")
	sellerRoutes.Use(authenticate)

	// Seller onboarding
	sellerRoutes.POST(
		"/applications",
		handler.CreateApplication,
	)

	sellerRoutes.GET(
		"/application",
		handler.GetMyApplication,
	)

	// Shop management
	sellerRoutes.GET(
		"/shop",
		handler.GetMyShop,
	)

	sellerRoutes.PATCH(
		"/shop",
		handler.UpdateMyShop,
	)

	// Seller dashboard
	sellerRoutes.GET(
		"/dashboard",
		handler.GetDashboard,
	)
}

// Admin routes must be protected by BOTH middlewares.
func RegisterAdminRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
	requireAdmin gin.HandlerFunc,
) {
	if authenticate == nil || requireAdmin == nil {
		panic("admin routes require authorization")
	}

	adminRoutes := api.Group("/admin")
	adminRoutes.Use(authenticate, requireAdmin)

	adminRoutes.POST(
		"/seller-applications/:id/decision",
		handler.DecideApplication,
	)
}
