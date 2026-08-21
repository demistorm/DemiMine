package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/demimine/manager/internal/api"
	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/backup"
	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/db"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/rcon"
	"github.com/demimine/manager/internal/scheduler"
	"github.com/demimine/manager/internal/spark"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
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

	if os.Getenv("RCON_PASSWORD") == "" {
		log.Fatal("RCON_PASSWORD environment variable is required")
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

	updateBundledPlugins(database, cfg)

	dockerClient.SyncServerStatus(context.Background(), database)

	// heal stale player rows — anyone left on a server that isn't running is a ghost
	if result, err := database.Exec("DELETE FROM players WHERE server_id IN (SELECT id FROM servers WHERE status != 'running')"); err != nil {
		log.Printf("Failed to prune players for non-running servers: %v", err)
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		log.Printf("Pruned %d stale player rows for non-running servers", rows)
	}

	startOnBoot(database, dockerClient, cfg)

	consoleManager := docker.NewConsoleManager(dockerClient, database)

	wsHandler := handlers.NewWSHandler(database, dockerClient, consoleManager, cfg)
	go wsHandler.Run()

	logManager := docker.NewLogManager(dockerClient, database, wsHandler.GetHub())
	rconPool := rcon.NewPool(os.Getenv("RCON_PASSWORD"))
	sparkService := spark.NewService(rconPool, wsHandler.GetHub(), logManager)
	eventManager := docker.NewEventManager(dockerClient, database, wsHandler.GetHub(), logManager, consoleManager, sparkService)

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
	defer backupScheduler.Stop()

	taskScheduler := scheduler.NewScheduler(database)
	router := api.NewRouter(database, cfg, dockerClient, consoleManager, wsHandler.GetHub(), backupManager, taskScheduler, sparkService)

	// Start schedulers AFTER handlers are set by NewRouter
	backupScheduler.Start()
	taskScheduler.Start()
	defer taskScheduler.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", wsHandler.Handle)
	mux.Handle("/", router)

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
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

	rows, err := database.Query("SELECT id, sanitized_name FROM servers WHERE status = 'running'")
	if err != nil {
		log.Printf("Error querying running servers: %v", err)
	} else {
		defer rows.Close()

		for rows.Next() {
			var id int64
			var sanitizedName string
			if err := rows.Scan(&id, &sanitizedName); err == nil {
				wg.Add(1)
				go func(serverID int64, serverSanitizedName string) {
					defer wg.Done()
					log.Printf("Stopping server: %s", serverSanitizedName)
					if err := dockerClient.StopContainer(ctx, serverSanitizedName, nil); err != nil {
						log.Printf("Failed to stop server %s: %v", serverSanitizedName, err)
					} else {
						if _, err := database.Exec("UPDATE servers SET status = 'stopped' WHERE id = ?", serverID); err != nil {
							log.Printf("Failed to update server %s status: %v", serverSanitizedName, err)
						}
					}
				}(id, sanitizedName)
			}
		}
	}

	proxyRows, err := database.Query("SELECT id, sanitized_name FROM proxies WHERE status = 'running'")
	if err != nil {
		log.Printf("Error querying running proxies: %v", err)
	} else {
		defer proxyRows.Close()

		for proxyRows.Next() {
			var id int64
			var sanitizedName string
			if err := proxyRows.Scan(&id, &sanitizedName); err == nil {
				wg.Add(1)
				go func(proxyID int64, proxySanitizedName string) {
					defer wg.Done()
					log.Printf("Stopping proxy: %s", proxySanitizedName)
					if err := dockerClient.StopProxyContainer(ctx, proxySanitizedName, nil); err != nil {
						log.Printf("Failed to stop proxy %s: %v", proxySanitizedName, err)
					} else {
						if _, err := database.Exec("UPDATE proxies SET status = 'stopped' WHERE id = ?", proxyID); err != nil {
							log.Printf("Failed to update proxy %s status: %v", proxySanitizedName, err)
						}
					}
				}(id, sanitizedName)
			}
		}
	}

	wg.Wait()
	log.Println("All servers and proxies stopped")
}

func startOnBoot(database *sql.DB, dockerClient *docker.Client, cfg *config.Config) {
	log.Println("Starting servers and proxies with start_on_boot enabled...")

	var wg sync.WaitGroup

	proxyRows, err := database.Query(
		"SELECT id, sanitized_name, host_port, COALESCE(ram_mb, 512) FROM proxies WHERE start_on_boot = 1")
	if err != nil {
		log.Printf("Error querying start_on_boot proxies: %v", err)
	} else {
		type proxyInfo struct {
			id            int64
			sanitizedName string
			hostPort      int
			ramMB         int
		}
		var proxies []proxyInfo
		for proxyRows.Next() {
			var p proxyInfo
			if err := proxyRows.Scan(&p.id, &p.sanitizedName, &p.hostPort, &p.ramMB); err != nil {
				log.Printf("Failed to scan proxy row: %v", err)
				continue
			}
			proxies = append(proxies, p)
		}
		proxyRows.Close()

		for _, p := range proxies {
			wg.Add(1)
			go func(p proxyInfo) {
				defer wg.Done()
				log.Printf("Starting proxy: %s", p.sanitizedName)
				proxyCfg := &docker.ProxyContainerConfig{
					Name:        p.sanitizedName,
					HostPort:    p.hostPort,
					ProxyPath:   filepath.Join(cfg.HostServersDir, p.sanitizedName),
					NetworkName: cfg.NetworkName,
					RAMMB:       p.ramMB,
				}
				if err := dockerClient.StartProxyContainer(context.Background(), p.sanitizedName, proxyCfg); err != nil {
					log.Printf("Failed to start proxy %s: %v", p.sanitizedName, err)
				} else {
					database.Exec("UPDATE proxies SET status = 'running' WHERE id = ?", p.id)
				}
			}(p)
		}
	}

	wg.Wait()

	serverRows, err := database.Query(
		"SELECT id, sanitized_name, type, version, ram_mb, host_port FROM servers WHERE start_on_boot = 1")
	if err != nil {
		log.Printf("Error querying start_on_boot servers: %v", err)
	} else {
		type serverInfo struct {
			id            int64
			sanitizedName string
			serverType    string
			version       string
			ramMB         int
			hostPort      sql.NullInt64
		}
		var servers []serverInfo
		for serverRows.Next() {
			var s serverInfo
			if err := serverRows.Scan(&s.id, &s.sanitizedName, &s.serverType, &s.version, &s.ramMB, &s.hostPort); err != nil {
				log.Printf("Failed to scan server row: %v", err)
				continue
			}
			servers = append(servers, s)
		}
		serverRows.Close()

		for _, s := range servers {
			wg.Add(1)
			go func(s serverInfo) {
				defer wg.Done()
				log.Printf("Starting server: %s", s.sanitizedName)
				port := 0
				if s.hostPort.Valid {
					port = int(s.hostPort.Int64)
				}
				serverCfg := &docker.ServerContainerConfig{
					Name:        s.sanitizedName,
					ServerType:  s.serverType,
					Version:     s.version,
					RAMMB:       s.ramMB,
					ServerPath:  filepath.Join(cfg.HostServersDir, s.sanitizedName),
					NetworkName: cfg.NetworkName,
					HostPort:    port,
				}
				if err := dockerClient.StartContainer(context.Background(), s.sanitizedName, serverCfg); err != nil {
					log.Printf("Failed to start server %s: %v", s.sanitizedName, err)
				} else {
					database.Exec("UPDATE servers SET status = 'running' WHERE id = ?", s.id)
				}
			}(s)
		}
	}

	wg.Wait()
	log.Println("All start_on_boot servers and proxies started")
}

func updateBundledPlugins(database *sql.DB, cfg *config.Config) {
	type pluginInfo struct {
		name        string
		resource    string
		globPattern string
	}

	plugins := []pluginInfo{
		{name: "DemiAuth", resource: "/app/resources/DemiAuth.jar", globPattern: "DemiAuth*.jar"},
		{name: "DemiDynamic", resource: "/app/resources/DemiDynamic.jar", globPattern: "DemiDynamic*.jar"},
	}

	rows, err := database.Query("SELECT sanitized_name FROM proxies")
	if err != nil {
		log.Printf("Plugin update: failed to query proxies: %v", err)
		return
	}
	var proxyNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			proxyNames = append(proxyNames, name)
		}
	}
	rows.Close()

	if len(proxyNames) == 0 {
		return
	}

	updated := 0
	for _, proxyName := range proxyNames {
		pluginsDir := filepath.Join(cfg.ServersDir, proxyName, "plugins")
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			log.Printf("Plugin update: failed to create plugins dir for %s: %v", proxyName, err)
			continue
		}

		for _, p := range plugins {
			matches, _ := filepath.Glob(filepath.Join(pluginsDir, p.globPattern))

			resourceInfo, err := os.Stat(p.resource)
			if err != nil {
				continue
			}

			targetName := p.name + ".jar"
			targetPath := filepath.Join(pluginsDir, targetName)
			needsUpdate := false

			if len(matches) == 0 {
				needsUpdate = true
			} else {
				for _, m := range matches {
					if filepath.Base(m) != targetName {
						os.Remove(m)
						needsUpdate = true
					} else {
						if fi, err := os.Stat(m); err == nil && fi.Size() != resourceInfo.Size() {
							os.Remove(m)
							needsUpdate = true
						}
					}
				}
			}

			if !needsUpdate {
				continue
			}

			src, err := os.Open(p.resource)
			if err != nil {
				log.Printf("Plugin update: failed to open %s: %v", p.resource, err)
				continue
			}

			dst, err := os.Create(targetPath)
			if err != nil {
				src.Close()
				log.Printf("Plugin update: failed to create %s: %v", targetPath, err)
				continue
			}

			_, err = io.Copy(dst, src)
			src.Close()
			dst.Close()
			if err != nil {
				log.Printf("Plugin update: failed to copy %s: %v", p.name, err)
				continue
			}

			log.Printf("Plugin update: updated %s for proxy %s", p.name, proxyName)
			updated++
		}
	}

	if updated > 0 {
		log.Printf("Plugin update: %d plugin(s) updated across %d proxy/ies", updated, len(proxyNames))
	}
}
