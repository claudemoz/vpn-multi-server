package device

import "github.com/gin-gonic/gin"

func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	devices := rg.Group("/device")

	devices.GET("/list", handler.List)
	devices.POST("", handler.Create)
	devices.GET("/:id", handler.Get)
	devices.GET("/:id/config", handler.Config)
	devices.PUT("/:id", handler.Update)
	devices.DELETE("/:id", handler.Delete)
	devices.POST("/:id/session", handler.StartSession)
	devices.GET("/:id/session", handler.CurrentSession)
}
