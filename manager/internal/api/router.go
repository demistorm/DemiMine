package api

import (
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/api/middleware"
	"github.com/demimine/manager/internal/backup"
	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/minimotd"
	"github.com/demimine/manager/internal/plugin"
	"github.com/demimine/manager/internal/scheduler"
	"github.com/demimine/manager/internal/spark"
	"github.com/demimine/manager/internal/websocket"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(database *sql.DB, cfg *config.Config, dockerClient *docker.Client, consoleManager *docker.ConsoleManager, hub *websocket.Hub, backupManager *backup.Manager, scheduler *scheduler.Scheduler, sparkService *spark.Service) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(middleware.CORS([]string{"*"}))

	rateLimiter := middleware.NewRateLimiter(500, 15*time.Minute)
	r.Use(rateLimiter.Middleware)

	pluginMgr := plugin.NewManager(database, cfg.ServersDir)
	minimotdMgr := minimotd.NewManager(cfg.ServersDir)
	sparkInstaller := spark.NewInstaller(pluginMgr, cfg.ServersDir)

	authHandler := handlers.NewAuthHandler(database, cfg.JWTSecret)
	serverHandler := handlers.NewServerHandler(database, dockerClient, consoleManager, cfg, minimotdMgr, sparkInstaller)
	proxyHandler := handlers.NewProxyHandler(database, dockerClient, consoleManager, cfg, pluginMgr, minimotdMgr, sparkInstaller)
	versionsHandler := handlers.NewVersionsHandler()
	javaHandler := handlers.NewJavaHandler()
	fileUploadHandler := handlers.NewFileUploadHandler(database, cfg)
	modrinthHandler := handlers.NewModrinthHandler()
	pluginHandler := handlers.NewPluginHandler(database, cfg.ServersDir)
	settingsHandler := handlers.NewSettingsHandler(database)
	jarUpdateHandler := handlers.NewJarUpdateHandler(database, cfg)
	playerHandler := handlers.NewPlayerHandler(database, dockerClient, cfg)
	apiKeyHandler := handlers.NewAPIKeyHandler(database)
	backupsHandler := handlers.NewBackupsHandler(database, backupManager, serverHandler, proxyHandler, hub, consoleManager)
	resourceHandler := handlers.NewResourceHandler(database, dockerClient, cfg)
	sparkHandler := spark.NewHandler(sparkService)
	sparkHandler.SetDB(database)

	if backupManager != nil {
		backupManager.SetHandlers(serverHandler, proxyHandler)
	}

	if scheduler != nil {
		scheduler.SetHandlers(serverHandler, proxyHandler)
	}

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/setup", authHandler.Setup)
			r.Post("/login", authHandler.Login)
			r.Post("/logout", authHandler.Logout)
			r.Get("/status", authHandler.Status)
		})

		r.Route("/versions", func(r chi.Router) {
			r.Get("/{type}", versionsHandler.List)
		})

		r.Route("/java", func(r chi.Router) {
			r.Get("/", javaHandler.List)
			r.Get("/required", javaHandler.GetRequired)
		})

		r.Route("/modrinth", func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))
			r.Get("/search", modrinthHandler.Search)
			r.Get("/project/{slug}", modrinthHandler.GetProject)
			r.Get("/project/{slug}/versions", modrinthHandler.GetVersions)
		})

		r.Route("/plugins", func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))
			r.Get("/{type}/{id}", pluginHandler.GetInstalled)
			r.Post("/{type}/{id}/install", pluginHandler.Install)
			r.Delete("/{type}/{id}/{project_id}", pluginHandler.Uninstall)
			r.Get("/{type}/{id}/updates", pluginHandler.CheckUpdates)
			r.Post("/{type}/{id}/{project_id}/update", pluginHandler.Update)
		})

		r.Route("/settings", func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))
			r.Get("/", settingsHandler.Get)
			r.Put("/", settingsHandler.Update)
			r.Post("/background-texture", settingsHandler.UploadBackgroundTexture)
			r.Get("/background-texture", settingsHandler.ServeBackgroundTexture)
			r.Delete("/background-texture", settingsHandler.DeleteBackgroundTexture)
			r.Post("/server-tile-texture", settingsHandler.UploadServerTileTexture)
			r.Get("/server-tile-texture", settingsHandler.ServeServerTileTexture)
			r.Delete("/server-tile-texture", settingsHandler.DeleteServerTileTexture)
			r.Post("/proxy-tile-texture", settingsHandler.UploadProxyTileTexture)
			r.Get("/proxy-tile-texture", settingsHandler.ServeProxyTileTexture)
			r.Delete("/proxy-tile-texture", settingsHandler.DeleteProxyTileTexture)
		})

		r.Route("/players", func(r chi.Router) {
			r.Post("/join", playerHandler.Join)
			r.Post("/leave", playerHandler.Leave)
		})

		r.Route("/api-keys", func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))
			r.Get("/", apiKeyHandler.List)
			r.Post("/", apiKeyHandler.Create)
			r.Delete("/{id}", apiKeyHandler.Delete)
		})

		r.Route("/backups", func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))
			r.Get("/", backupsHandler.List)
			r.Post("/", backupsHandler.Create)
			r.Get("/{id}/download", backupsHandler.Download)
			r.Post("/restore", backupsHandler.RestoreFromUpload)
			r.Post("/{id}/restore", backupsHandler.RestoreFromDisk)
			r.Delete("/{id}", backupsHandler.Delete)
		})

		r.Get("/servers/auto-shutdown", playerHandler.GetAutoShutdownServers)
		r.Get("/servers/{name}/status", playerHandler.GetServerStatus)
		r.Post("/servers/{name}/start-by-name", playerHandler.StartServerByName)
		r.Post("/servers/{name}/stop-by-name", playerHandler.StopServerByName)
		r.Post("/servers/{name}/can-start", resourceHandler.CanStartServer)
		r.Post("/servers/{name}/wait-for-removal", resourceHandler.WaitForContainerRemoval)

		r.Get("/system/resources", resourceHandler.GetSystemResources)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))

			r.Route("/servers", func(r chi.Router) {
				r.Get("/", serverHandler.List)
				r.Post("/", serverHandler.Create)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", serverHandler.Get)
					r.Patch("/", serverHandler.Update)
					r.Delete("/", serverHandler.Delete)
					r.Post("/start", serverHandler.Start)
					r.Post("/stop", serverHandler.Stop)
					r.Post("/restart", serverHandler.Restart)

					r.Get("/logs", serverHandler.GetLogs)
					r.Post("/command", serverHandler.ExecuteCommand)
					r.Get("/history", serverHandler.GetCommandHistory)

					r.Route("/files", func(r chi.Router) {
						r.Get("/", serverHandler.ListFiles)
						r.Get("/content", serverHandler.GetFileContent)
						r.Put("/content", serverHandler.WriteFileContent)
						r.Delete("/", serverHandler.DeleteFile)
						r.Get("/download", serverHandler.DownloadFile)
						r.Post("/rename", serverHandler.RenameFile)
						r.Post("/upload", fileUploadHandler.Upload)
					})

					r.Get("/jar-update", jarUpdateHandler.CheckServerJarUpdate)
					r.Post("/jar-update", jarUpdateHandler.UpdateServerJar)
					r.Get("/players", playerHandler.GetServerPlayers)

					r.Post("/spark/profile/start", sparkHandler.StartServerProfiler)
					r.Post("/spark/profile/stop", sparkHandler.StopServerProfiler)
				})

				r.Post("/{id}/icon", serverHandler.UploadIcon)
				r.Delete("/{id}/icon", serverHandler.DeleteIcon)
			})

			r.Route("/proxies", func(r chi.Router) {
				r.Get("/", proxyHandler.List)
				r.Post("/", proxyHandler.Create)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", proxyHandler.Get)
					r.Patch("/", proxyHandler.Update)
					r.Delete("/", proxyHandler.Delete)
					r.Post("/start", proxyHandler.Start)
					r.Post("/stop", proxyHandler.Stop)
					r.Post("/restart", proxyHandler.Restart)
					r.Get("/logs", proxyHandler.GetLogs)
					r.Post("/command", proxyHandler.ExecuteCommand)

					r.Route("/files", func(r chi.Router) {
						r.Get("/", proxyHandler.ListFiles)
						r.Get("/content", proxyHandler.GetFileContent)
						r.Put("/content", proxyHandler.WriteFileContent)
						r.Delete("/", proxyHandler.DeleteFile)
						r.Get("/download", proxyHandler.DownloadFile)
						r.Post("/rename", proxyHandler.RenameFile)
						r.Post("/upload", fileUploadHandler.UploadProxy)
					})

					r.Get("/jar-update", jarUpdateHandler.CheckProxyJarUpdate)
					r.Post("/jar-update", jarUpdateHandler.UpdateProxyJar)

					r.Post("/spark/profile/start", sparkHandler.StartProxyProfiler)
					r.Post("/spark/profile/stop", sparkHandler.StopProxyProfiler)
				})

				r.Post("/{id}/icon", proxyHandler.UploadIcon)
				r.Delete("/{id}/icon", proxyHandler.DeleteIcon)
			})
		})
	})

	r.Get("/api/servers/{id}/icon", serverHandler.GetIcon)
	r.Get("/api/proxies/{id}/icon", proxyHandler.GetIcon)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	webuiPath := "/app/webui/build"
	if _, err := os.Stat(webuiPath); err == nil {
		fs := http.FileServer(http.Dir(webuiPath))
		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path == "/health" {
				http.NotFound(w, r)
				return
			}

			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}

			filePath := webuiPath + path
			if _, err := os.Stat(filePath); err == nil {
				fs.ServeHTTP(w, r)
				return
			}

			r.URL.Path = "/"
			fs.ServeHTTP(w, r)
		}))
	}

	return r
}
