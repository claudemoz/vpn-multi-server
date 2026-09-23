package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	Env  string
	Port string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	TimeZone string
}

type JWTConfig struct {
	Secret          string
	ExpirationHours int
}

type WireGuardConfig struct {
	Subnet     string
	DNS        string
	AllowedIPs string
	Keepalive  int
}

// AgentConfig drives the gRPC communication between the control plane and the
// agents running on the VPN servers.
type AgentConfig struct {
	// Token is the shared secret sent as "authorization: Bearer <token>".
	Token string
	// TLSCAFile enables TLS when set: the CA used to verify agent certificates.
	// When empty the control plane dials agents in plaintext (development).
	TLSCAFile string
	// TLSServerName overrides the expected server name in agent certificates.
	TLSServerName string
	DialTimeout   time.Duration
	CallTimeout   time.Duration
	// SyncEnabled starts the background reconciler that keeps every agent in
	// sync with the database and collects handshakes.
	SyncEnabled  bool
	SyncInterval time.Duration
}

type Config struct {
	App       AppConfig
	Database  DatabaseConfig
	JWT       JWTConfig
	WireGuard WireGuardConfig
	Agent     AgentConfig
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, falling back to system environment")
	}

	return &Config{
		App: AppConfig{
			Env:  getEnv("APP_ENV", "development"),
			Port: getEnv("APP_PORT", "8080"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "wg-vpn-platform"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			TimeZone: getEnv("DB_TIMEZONE", "UTC"),
		},
		JWT: JWTConfig{
			Secret:          getEnv("JWT_SECRET", "change_me"),
			ExpirationHours: getEnvAsInt("JWT_EXPIRATION_HOURS", 2160), // 90 days (~3 months)
		},
		WireGuard: WireGuardConfig{
			Subnet:     getEnv("WG_DEFAULT_SUBNET", "10.8.0.0/24"),
			DNS:        getEnv("WG_DNS", "1.1.1.1,8.8.8.8"),
			AllowedIPs: getEnv("WG_ALLOWED_IPS", "0.0.0.0/0,::/0"),
			Keepalive:  getEnvAsInt("WG_KEEPALIVE", 25),
		},
		Agent: AgentConfig{
			Token:         getEnv("AGENT_TOKEN", ""),
			TLSCAFile:     getEnv("AGENT_TLS_CA", ""),
			TLSServerName: getEnv("AGENT_TLS_SERVER_NAME", ""),
			DialTimeout:   getEnvAsDuration("AGENT_DIAL_TIMEOUT", 5*time.Second),
			CallTimeout:   getEnvAsDuration("AGENT_CALL_TIMEOUT", 10*time.Second),
			SyncEnabled:   getEnvAsBool("AGENT_SYNC_ENABLED", true),
			SyncInterval:  getEnvAsDuration("AGENT_SYNC_INTERVAL", 30*time.Second),
		},
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode, d.TimeZone,
	)
}
