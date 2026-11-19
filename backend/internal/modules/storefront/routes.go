
package storefront

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

// RegisterRoutes registers public storefront endpoints.
func RegisterRoutes(rg *gin.RouterGroup) {
    storefrontGroup := rg.Group("/storefront")

    storefrontGroup.GET("/health", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "status":  "ok",
            "module":  "storefront",
            "message": "Storefront API is running",
        })
    })
}
