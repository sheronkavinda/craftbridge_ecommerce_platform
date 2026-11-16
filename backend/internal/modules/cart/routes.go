package cart

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("cart requires authentication middleware")
	}

	group := api.Group("/me/cart")
	group.Use(authenticate)

	group.GET("", handler.Get)
	group.POST("/items", handler.Add)
	group.PATCH("/items/:id", handler.Update)
	group.DELETE("/items/:id", handler.Remove)
}
