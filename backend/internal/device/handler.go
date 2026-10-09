package device

import (
	apperrors "backend/pkg/errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	devices, err := h.service.GetAll()
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, devices)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	device, err := h.service.GetByID(id)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, device)
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return
	}

	var req CreateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	device, err := h.service.Create(c.Request.Context(), userID.(uuid.UUID), req)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusCreated, device)
}

// Config returns the wg-quick client configuration as plain text.
func (h *Handler) Config(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	cfg, err := h.service.ClientConfig(id)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	c.Header("Content-Disposition", `attachment; filename="wg-`+id.String()[:8]+`.conf"`)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(cfg))
}

func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	var req UpdateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	device, err := h.service.Update(c.Request.Context(), id, req)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, device)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, id, ok := ownerAndID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, id); err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) StartSession(c *gin.Context) {
	userID, id, ok := ownerAndID(c)
	if !ok {
		return
	}

	session, err := h.service.StartSession(userID, id)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *Handler) CurrentSession(c *gin.Context) {
	userID, id, ok := ownerAndID(c)
	if !ok {
		return
	}

	session, err := h.service.CurrentSession(userID, id)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// ownerAndID reads the authenticated user and the :id param, writing the
// error response itself when either is missing or invalid.
func ownerAndID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return uuid.Nil, uuid.Nil, false
	}

	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return uuid.Nil, uuid.Nil, false
	}
	return userID.(uuid.UUID), id, true
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperrors.NewInvalidInput("invalid device id")
	}
	return id, nil
}
