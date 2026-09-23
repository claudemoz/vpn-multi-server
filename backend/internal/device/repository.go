package device

import (
	apperrors "backend/pkg/errors"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(device *Device) error {
	if err := r.db.Create(device).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.NewConflict("public_key already in use, or assigned_ip already taken on this server")
		}
		return err
	}
	return nil
}

func (r *Repository) FindAll() ([]Device, error) {
	var devices []Device
	if err := r.db.Order("created_at ASC").Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (r *Repository) FindByID(id uuid.UUID) (*Device, error) {
	var device Device
	if err := r.db.First(&device, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFound("device not found")
		}
		return nil, err
	}
	return &device, nil
}

// FindAssignedIPsByServer returns every IP assigned on a server, including
// soft-deleted devices, because the unique constraint still applies to them.
func (r *Repository) FindAssignedIPsByServer(serverID uuid.UUID) ([]string, error) {
	var ips []string
	err := r.db.Unscoped().
		Model(&Device{}).
		Where("server_id = ?", serverID).
		Pluck("assigned_ip", &ips).Error
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// FindActiveByServer returns the devices that must be configured as peers on
// a server: not soft-deleted and not revoked.
func (r *Repository) FindActiveByServer(serverID uuid.UUID) ([]Device, error) {
	var devices []Device
	err := r.db.
		Where("server_id = ? AND revoked_at IS NULL", serverID).
		Order("created_at ASC").
		Find(&devices).Error
	if err != nil {
		return nil, err
	}
	return devices, nil
}

// UpdateLastHandshakes stores the handshake times reported by an agent,
// keyed by device public key. Unknown keys are ignored.
func (r *Repository) UpdateLastHandshakes(serverID uuid.UUID, handshakes map[string]time.Time) error {
	if len(handshakes) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for publicKey, at := range handshakes {
			err := tx.Model(&Device{}).
				Where("server_id = ? AND public_key = ?", serverID, publicKey).
				Update("last_handshake", at).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) Update(device *Device) error {
	if err := r.db.Save(device).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.NewConflict("public_key already in use, or assigned_ip already taken on this server")
		}
		return err
	}
	return nil
}

func (r *Repository) Delete(id uuid.UUID) error {
	result := r.db.Delete(&Device{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.NewNotFound("device not found")
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
