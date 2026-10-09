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

func (r *Repository) FindByUserID(userID uuid.UUID) ([]Device, error) {
	var devices []Device
	if err := r.db.Where("user_id = ?", userID).Find(&devices).Error; err != nil {
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

// Delete removes the row for good (not a soft delete) so its assigned_ip and
// public_key, both under a unique constraint, can be reused right away. The
// device's open session, if any, is closed in the same transaction.
func (r *Repository) Delete(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&Session{}).
			Where("device_id = ? AND disconnected_at IS NULL", id).
			Update("disconnected_at", time.Now()).Error
		if err != nil {
			return err
		}

		result := tx.Unscoped().Delete(&Device{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return apperrors.NewNotFound("device not found")
		}
		return nil
	})
}

func (r *Repository) DeleteAllForUser(userID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&Device{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&Session{}).Error
	})
}

// FindOpenSession returns the session of a device that has not been closed.
func (r *Repository) FindOpenSession(deviceID uuid.UUID) (*Session, error) {
	var session Session
	err := r.db.
		Where("device_id = ? AND disconnected_at IS NULL", deviceID).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFound("no open session for this device")
		}
		return nil, err
	}
	return &session, nil
}

// StartSession opens a session for the device, or returns the one already
// open so that retries do not reset the connection time.
func (r *Repository) StartSession(device *Device) (*Session, error) {
	if open, err := r.FindOpenSession(device.ID); err == nil {
		return open, nil
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}

	session := &Session{
		UserID:      device.UserID,
		ServerID:    device.ServerID,
		DeviceID:    device.ID,
		AssignedIP:  device.AssignedIP,
		ConnectedAt: time.Now(),
	}
	if err := r.db.Create(session).Error; err != nil {
		if isUniqueViolation(err) {
			return r.FindOpenSession(device.ID)
		}
		return nil, err
	}
	return session, nil
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
