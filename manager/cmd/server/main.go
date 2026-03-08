package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/demimine/manager/internal/api"
	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/db"
	"github.com/demimine/manager/internal/docker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	if err := os.MkdirAll(cfg.ServersDir, 0755); err != nil {
		log.Fatalf("Failed to create servers directory: %v", err)
	}
	if err := os.MkdirAll(cfg.ProxiesDir, 0755); err != nil {
		log.Fatalf("Failed to create proxies directory: %v", err)
	}
	if err := os.MkdirAll(cfg.BackupsDir, 0755); err != nil {
		log.Fatalf("Failed to create backups directory: %v", err)
	}
	if err := os.MkdirAll(cfg.JavaDir, 0755); err != nil {
		log.Fatalf("Failed to create java directory: %v", err)
	}

	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Database migrations completed")

	dockerClient, err := docker.NewClient(cfg.NetworkName)
	if err != nil {
		log.Fatalf("Failed to create docker client: %v", err)
	}
	defer dockerClient.Close()

	if dockerClient == nil {
		log.Fatal("Docker client is nil after successful initialization")
	}
	log.Println("Docker client initialized successfully")

	consoleManager := docker.NewConsoleManager(dockerClient, database)

	wsHandler := handlers.NewWSHandler(database, dockerClient, consoleManager, cfg)
	go wsHandler.Run()

	logManager := docker.NewLogManager(dockerClient, database, wsHandler.GetHub())
	eventManager := docker.NewEventManager(dockerClient, database, wsHandler.GetHub(), logManager, consoleManager)

	if err := logManager.StartStreamingForRunningContainers(context.Background(), database); err != nil {
		log.Printf("Failed to start log streaming for running containers: %v", err)
	}

	if err := consoleManager.StartConsolesForRunningContainers(context.Background()); err != nil {
		log.Printf("Failed to start console streaming for running containers: %v", err)
	}

	go eventManager.Start(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", wsHandler.Handle)

	router := api.NewRouter(database, cfg, dockerClient, consoleManager)
	mux.Handle("/", router)

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting server on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
