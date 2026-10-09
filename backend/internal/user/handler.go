package user

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
	users, err := h.service.GetAll()
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	user, err := h.service.GetByID(id)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) GetMe(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return
	}

	user, err := h.service.GetByID(userID.(uuid.UUID))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	user, err := h.service.Update(id, req)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) UpdateMe(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	user, err := h.service.Update(userID.(uuid.UUID), req)
	if err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		apperrors.Handle(c, err)
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	if err := h.service.ChangePassword(userID.(uuid.UUID), req); err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteMe(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		apperrors.Handle(c, apperrors.NewUnauthorized("unauthenticated"))
		return
	}

	var req DeleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Handle(c, apperrors.NewInvalidInput(err.Error()))
		return
	}

	if err := h.service.DeleteWithPassword(c.Request.Context(), userID.(uuid.UUID), req.Password); err != nil {
		apperrors.Handle(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperrors.NewInvalidInput("invalid user id")
	}
	return id, nil
}
