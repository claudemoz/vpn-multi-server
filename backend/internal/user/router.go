package user

import "github.com/gin-gonic/gin"

func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	users := rg.Group("/user")

	users.GET("/me", handler.GetMe)
	users.PUT("/me", handler.UpdateMe)
	users.PUT("/me/password", handler.ChangePassword)
	users.DELETE("/me", handler.DeleteMe)

	users.GET("/list", handler.List)
	users.GET("/:id", handler.Get)
	users.PUT("/:id", handler.Update)
	users.DELETE("/:id", handler.Delete)
}
