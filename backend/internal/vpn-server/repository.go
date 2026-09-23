package server

import (
	apperrors "backend/pkg/errors"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(server *Server) error {
	if err := r.db.Create(server).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.NewConflict("wg_public_key already in use")
		}
		return err
	}
	return nil
}

func (r *Repository) FindAll() ([]Server, error) {
	var servers []Server
	if err := r.db.Order("created_at ASC").Find(&servers).Error; err != nil {
		return nil, err
	}
	return servers, nil
}

func (r *Repository) FindByID(id uuid.UUID) (*Server, error) {
	var server Server
	if err := r.db.First(&server, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFound("server not found")
		}
		return nil, err
	}
	return &server, nil
}

func (r *Repository) Update(server *Server) error {
	if err := r.db.Save(server).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.NewConflict("wg_public_key already in use")
		}
		return err
	}
	return nil
}

// UpdateCurrentPeers stores the peer count observed on the server's agent
// without touching the rest of the row.
func (r *Repository) UpdateCurrentPeers(id uuid.UUID, count int) error {
	return r.db.Model(&Server{}).
		Where("id = ?", id).
		Update("current_peers", count).Error
}

func (r *Repository) Delete(id uuid.UUID) error {
	result := r.db.Delete(&Server{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewNotFound("server not found")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "UNIQUE constraint")
}
