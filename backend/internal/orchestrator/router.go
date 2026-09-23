package orchestrator

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the agent endpoints next to the server CRUD:
//
//	GET  /server/:id/agent/status  health + live interface state
//	GET  /server/:id/agent/peers   peers configured on the agent
//	POST /server/:id/agent/sync    force a reconciliation
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	agent := rg.Group("/server/:id/agent")

	agent.GET("/status", handler.Status)
	agent.GET("/peers", handler.Peers)
	agent.POST("/sync", handler.Sync)
}
