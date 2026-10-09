package device

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (Session) TableName() string {
	return "vpn_session"
}

// Session is one VPN connection of a device, from tunnel up to disconnect.
type Session struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	ServerID       uuid.UUID  `gorm:"column:server_id;type:uuid;not null"`
	DeviceID       uuid.UUID  `gorm:"column:device_id;type:uuid;not null"`
	AssignedIP     string     `gorm:"column:assigned_ip;size:45;not null"`
	ConnectedAt    time.Time  `gorm:"column:connected_at;not null"`
	DisconnectedAt *time.Time `gorm:"column:disconnected_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (s *Session) BeforeCreate(_ *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

type SessionResponse struct {
	ID             string  `json:"id"`
	DeviceID       string  `json:"device_id"`
	ServerID       string  `json:"server_id"`
	AssignedIP     string  `json:"assigned_ip"`
	ConnectedAt    string  `json:"connected_at"`
	DisconnectedAt *string `json:"disconnected_at,omitempty"`
	// DurationSeconds runs up to now for an open session.
	DurationSeconds int64 `json:"duration_seconds"`
}

func ToSessionResponse(s Session) SessionResponse {
	end := time.Now()
	if s.DisconnectedAt != nil {
		end = *s.DisconnectedAt
	}
	return SessionResponse{
		ID:              s.ID.String(),
		DeviceID:        s.DeviceID.String(),
		ServerID:        s.ServerID.String(),
		AssignedIP:      s.AssignedIP,
		ConnectedAt:     s.ConnectedAt.UTC().Format(time.RFC3339),
		DisconnectedAt:  formatTimePtr(s.DisconnectedAt),
		DurationSeconds: int64(end.Sub(s.ConnectedAt).Seconds()),
	}
}
