package errors

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidInput = errors.New("invalid input")
	ErrForbidden    = errors.New("forbidden")
	ErrUnavailable  = errors.New("upstream unavailable")
)

type AppError struct {
	Err        error
	Message    string
	StatusCode int
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Err.Error()
}

func NewNotFound(msg string) *AppError {
	return &AppError{Err: ErrNotFound, Message: msg, StatusCode: http.StatusNotFound}
}

func NewConflict(msg string) *AppError {
	return &AppError{Err: ErrConflict, Message: msg, StatusCode: http.StatusConflict}
}

func NewUnauthorized(msg string) *AppError {
	return &AppError{Err: ErrUnauthorized, Message: msg, StatusCode: http.StatusUnauthorized}
}

func NewInvalidInput(msg string) *AppError {
	return &AppError{Err: ErrInvalidInput, Message: msg, StatusCode: http.StatusBadRequest}
}

func NewForbidden(msg string) *AppError {
	return &AppError{Err: ErrForbidden, Message: msg, StatusCode: http.StatusForbidden}
}

// NewUnavailable signals that a dependency (e.g. a remote agent) could not be
// reached; the request may succeed if retried later.
func NewUnavailable(msg string) *AppError {
	return &AppError{Err: ErrUnavailable, Message: msg, StatusCode: http.StatusServiceUnavailable}
}

func Handle(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.StatusCode, gin.H{"error": appErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}
