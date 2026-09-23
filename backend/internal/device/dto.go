package device

type CreateDeviceRequest struct {
	ServerID string `json:"server_id" binding:"required,uuid"`
	Name     string `json:"name"      binding:"required,min=1,max=100"`
	// PublicKey is optional: when omitted the platform generates a key pair and
	// returns the private key once in the creation response.
	PublicKey string `json:"public_key" binding:"omitempty,max=64"`
	// AssignedIP is optional: when omitted an address is allocated from the VPN subnet.
	AssignedIP string `json:"assigned_ip" binding:"omitempty,max=45"`
}

type UpdateDeviceRequest struct {
	ServerID   *string `json:"server_id"   binding:"omitempty,uuid"`
	Name       *string `json:"name"        binding:"omitempty,min=1,max=100"`
	PublicKey  *string `json:"public_key"  binding:"omitempty,max=64"`
	AssignedIP *string `json:"assigned_ip" binding:"omitempty,max=45"`
}

type DeviceResponse struct {
	ID            string  `json:"id"`
	UserID        string  `json:"user_id"`
	ServerID      string  `json:"server_id"`
	Name          string  `json:"name"`
	PublicKey     string  `json:"public_key"`
	AssignedIP    string  `json:"assigned_ip"`
	LastHandshake *string `json:"last_handshake,omitempty"`
	RevokedAt     *string `json:"revoked_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// CreateDeviceResponse is returned once at creation. PrivateKey is only set when
// the platform generated the key pair; it is never stored.
type CreateDeviceResponse struct {
	DeviceResponse
	PrivateKey string `json:"private_key,omitempty"`
	Config     string `json:"config"`
	// AgentSynced is true when the peer was pushed to the server's agent
	// immediately. When false the reconciler will apply it shortly; the
	// tunnel may not work until then.
	AgentSynced bool `json:"agent_synced"`
}
