package app

import (
	"context"
	"log"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/device"
	"backend/internal/orchestrator"
	"backend/internal/user"
	vpnserver "backend/internal/vpn-server"
	"backend/internal/wireguard"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type App struct {
	Router *gin.Engine
	DB     *gorm.DB

	orchestrator *orchestrator.Orchestrator
	reconciler   *orchestrator.Reconciler
}

func New(cfg *config.Config) *App {
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	wgSettings, err := wireguard.NewSettings(
		cfg.WireGuard.Subnet,
		cfg.WireGuard.DNS,
		cfg.WireGuard.AllowedIPs,
		cfg.WireGuard.Keepalive,
		cfg.WireGuard.MTU,
		cfg.WireGuard.IPv6Prefix,
	)
	if err != nil {
		log.Fatalf("invalid wireguard configuration: %v", err)
	}

	// Repositories
	userRepo := user.NewRepository(db)
	serverRepo := vpnserver.NewRepository(db)
	deviceRepo := device.NewRepository(db)

	// Services
	serverService := vpnserver.NewService(serverRepo, cfg.WireGuard.Subnet)

	// gRPC link to the agents: the orchestrator pushes peers to each server
	// and the reconciler keeps them converged in the background.
	dialer, err := orchestrator.NewDialer(cfg.Agent)
	if err != nil {
		log.Fatalf("invalid agent configuration: %v", err)
	}
	if cfg.Agent.Token == "" {
		log.Println("WARNING: AGENT_TOKEN is empty, agents will reject calls unless started with AGENT_INSECURE_NO_AUTH=true")
	}
	orch := orchestrator.New(
		orchestrator.NewPool(dialer),
		orchestrator.NewServerSource(serverService),
		orchestrator.NewPeerSource(deviceRepo, wgSettings),
	)

	deviceService := device.NewService(deviceRepo, serverService, wgSettings, orch)
	userService := user.NewService(userRepo, deviceService)
	authService := auth.NewService(userService, cfg.JWT)

	// Handlers
	userHandler := user.NewHandler(userService)
	serverHandler := vpnserver.NewHandler(serverService)
	deviceHandler := device.NewHandler(deviceService)
	authHandler := auth.NewHandler(authService)
	agentHandler := orchestrator.NewHandler(orch)

	// Routes
	router := gin.Default()
	if cfg.App.Env != "production" {
		router.Use(corsMiddleware())
	}
	router.GET("/health", healthCheck)

	v1 := router.Group("/api/v1")
	auth.RegisterRoutes(v1, authHandler)

	protected := v1.Group("")
	protected.Use(auth.Middleware(authService))

	user.RegisterRoutes(protected, userHandler)
	vpnserver.RegisterRoutes(protected, serverHandler)
	device.RegisterRoutes(protected, deviceHandler)
	orchestrator.RegisterRoutes(protected, agentHandler)

	app := &App{
		Router:       router,
		DB:           db,
		orchestrator: orch,
	}
	if cfg.Agent.SyncEnabled {
		app.reconciler = orchestrator.NewReconciler(orch, cfg.Agent.SyncInterval, log.Default())
	}
	return app
}

// Start launches background workers (the agent reconciler). It returns
// immediately; workers stop when ctx is cancelled.
func (a *App) Start(ctx context.Context) {
	if a.reconciler != nil {
		go a.reconciler.Run(ctx)
	}
}

// Close releases resources held by the app (agent connections).
func (a *App) Close() error {
	return a.orchestrator.Close()
}
