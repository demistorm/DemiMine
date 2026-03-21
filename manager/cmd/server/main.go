package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/demimine/manager/internal/api"
	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/backup"
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

	dockerClient.SyncServerStatus(context.Background(), database)

	consoleManager := docker.NewConsoleManager(dockerClient, database)

	wsHandler := handlers.NewWSHandler(database, dockerClient, consoleManager, cfg)
	go wsHandler.Run()

	logManager := docker.NewLogManager(dockerClient, database, wsHandler.GetHub())
	eventManager := docker.NewEventManager(dockerClient, database, wsHandler.GetHub(), logManager, consoleManager)

	if err := logManager.StartStreamingForRunningContainers(context.Background(), database); err != nil {
		log.Printf("Failed to start log streaming for running containers: %v", err)
	}

	if err := logManager.StartStreamingForRunningProxies(context.Background()); err != nil {
		log.Printf("Failed to start log streaming for running proxies: %v", err)
	}

	if err := consoleManager.StartConsolesForRunningContainers(context.Background()); err != nil {
		log.Printf("Failed to start console streaming for running containers: %v", err)
	}

	go eventManager.Start(context.Background())

	backupManager := backup.NewManager(database, cfg.DataDir, cfg.BackupsDir, cfg.ServersDir, cfg.JavaDir)
	backupScheduler := backup.NewScheduler(backupManager)
	backupScheduler.Start()
	defer backupScheduler.Stop()

	router := api.NewRouter(database, cfg, dockerClient, consoleManager, wsHandler.GetHub(), backupManager)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", wsHandler.Handle)
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

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	gracefulShutdown(database, dockerClient, ctx)

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

func gracefulShutdown(database *sql.DB, dockerClient *docker.Client, ctx context.Context) {
	log.Println("Stopping all servers and proxies gracefully...")

	var wg sync.WaitGroup

	rows, err := database.Query("SELECT id, name FROM servers WHERE status = 'running'")
	if err != nil {
		log.Printf("Error querying running servers: %v", err)
	} else {
		defer rows.Close()

		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err == nil {
				wg.Add(1)
				go func(serverID int64, serverName string) {
					defer wg.Done()
					log.Printf("Stopping server: %s", serverName)
					if err := dockerClient.StopContainer(ctx, serverName, nil); err != nil {
						log.Printf("Failed to stop server %s: %v", serverName, err)
					} else {
						if _, err := database.Exec("UPDATE servers SET status = 'stopped' WHERE id = ?", serverID); err != nil {
							log.Printf("Failed to update server %s status: %v", serverName, err)
						}
					}
				}(id, name)
			}
		}
	}

	proxyRows, err := database.Query("SELECT id, name FROM proxies WHERE status = 'running'")
	if err != nil {
		log.Printf("Error querying running proxies: %v", err)
	} else {
		defer proxyRows.Close()

		for proxyRows.Next() {
			var id int64
			var name string
			if err := proxyRows.Scan(&id, &name); err == nil {
				wg.Add(1)
				go func(proxyID int64, proxyName string) {
					defer wg.Done()
					log.Printf("Stopping proxy: %s", proxyName)
					if err := dockerClient.StopProxyContainer(ctx, proxyName, nil); err != nil {
						log.Printf("Failed to stop proxy %s: %v", proxyName, err)
					} else {
						if _, err := database.Exec("UPDATE proxies SET status = 'stopped' WHERE id = ?", proxyID); err != nil {
							log.Printf("Failed to update proxy %s status: %v", proxyName, err)
						}
					}
				}(id, name)
			}
		}
	}

	wg.Wait()
	log.Println("All servers and proxies stopped")
}
