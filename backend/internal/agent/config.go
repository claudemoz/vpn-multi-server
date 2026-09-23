package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	BackendWGCtrl = "wgctrl"
	BackendMemory = "memory"
)

// Config holds the runtime settings of an agent. Everything comes from the
// environment so the same binary can be dropped on any VPN server.
type Config struct {
	// ListenAddr is the gRPC listen address, e.g. ":50051".
	ListenAddr string
	// Interface is the WireGuard interface managed by this agent, e.g. "wg0".
	Interface string
	// Backend selects the wireguard.Client implementation: "wgctrl" (real
	// kernel/userspace device) or "memory" (development, no root needed).
	Backend string
	// Token is the shared secret the control plane must present in the
	// "authorization: Bearer <token>" metadata of every call.
	Token string
	// InsecureNoAuth disables token verification. Development only.
	InsecureNoAuth bool
	// TLSCertFile / TLSKeyFile enable TLS on the listener when both are set.
	TLSCertFile string
	TLSKeyFile  string
	// ShutdownTimeout bounds the graceful stop on SIGINT/SIGTERM.
	ShutdownTimeout time.Duration
}

// LoadConfig reads the agent configuration from the environment (and an
// optional .env file in the working directory).
func LoadConfig() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		ListenAddr:      getEnv("AGENT_LISTEN", ":50051"),
		Interface:       getEnv("WG_INTERFACE", "wg0"),
		Backend:         getEnv("AGENT_BACKEND", BackendWGCtrl),
		Token:           getEnv("AGENT_TOKEN", ""),
		InsecureNoAuth:  getEnvBool("AGENT_INSECURE_NO_AUTH", false),
		TLSCertFile:     getEnv("AGENT_TLS_CERT", ""),
		TLSKeyFile:      getEnv("AGENT_TLS_KEY", ""),
		ShutdownTimeout: getEnvDuration("AGENT_SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	if c.Backend != BackendWGCtrl && c.Backend != BackendMemory {
		return fmt.Errorf("AGENT_BACKEND must be %q or %q, got %q", BackendWGCtrl, BackendMemory, c.Backend)
	}
	if c.Token == "" && !c.InsecureNoAuth {
		return errors.New("AGENT_TOKEN is required (or set AGENT_INSECURE_NO_AUTH=true for local development)")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return errors.New("AGENT_TLS_CERT and AGENT_TLS_KEY must be set together")
	}
	return nil
}

// TLSEnabled reports whether the listener should use TLS.
func (c Config) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
