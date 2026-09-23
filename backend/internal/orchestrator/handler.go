package orchestrator

import (
	"errors"
	"net/http"

	apperrors "backend/pkg/errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler exposes agent operations over HTTP under /server/:id/agent.
type Handler struct {
	orch *Orchestrator
}

func NewHandler(orch *Orchestrator) *Handler {
	return &Handler{orch: orch}
}

// Status returns the agent health and the live interface state.
func (h *Handler) Status(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	status, err := h.orch.Status(c.Request.Context(), id)
	if err != nil {
		apperrors.Handle(c, toAppError(err))
		return
	}
	c.JSON(http.StatusOK, status)
}

// Peers returns the peers configured on the agent right now.
func (h *Handler) Peers(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	peers, err := h.orch.LivePeers(c.Request.Context(), id)
	if err != nil {
		apperrors.Handle(c, toAppError(err))
		return
	}
	c.JSON(http.StatusOK, peers)
}

// Sync forces a full reconciliation of one server.
func (h *Handler) Sync(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	result, err := h.orch.SyncServer(c.Request.Context(), id)
	if err != nil {
		apperrors.Handle(c, toAppError(err))
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperrors.NewInvalidInput("invalid server id")
	}
	return id, nil
}

// toAppError maps orchestrator errors to HTTP-aware errors. Errors that are
// already AppErrors (e.g. server not found) pass through untouched.
func toAppError(err error) error {
	var appErr *apperrors.AppError
	switch {
	case errors.As(err, &appErr):
		return err
	case errors.Is(err, ErrNoEndpoint):
		return apperrors.NewInvalidInput("server has no grpc_endpoint configured")
	case errors.Is(err, ErrAgentUnavailable):
		return apperrors.NewUnavailable(err.Error())
	default:
		return apperrors.NewUnavailable("agent error: " + err.Error())
	}
}
