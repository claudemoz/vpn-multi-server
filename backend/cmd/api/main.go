package main

import (
	"backend/internal/app"
	"backend/internal/config"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
)

func main() {
	cfg := config.Load()
	app := app.New(cfg)
	defer app.Close()

	workersCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	app.Start(workersCtx)

	srv := &http.Server{
		Addr:              ":" + cfg.App.Port,
		Handler:           app.Router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	c := color.New(color.FgGreen).Add(color.Bold)
	logger := log.New(os.Stdout, "", log.LstdFlags)
	go func() {
		logger.Print(c.Sprintf("Api listening on :%s", cfg.App.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutdown signal received")
	stopWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
