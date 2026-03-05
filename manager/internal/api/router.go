package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/api/middleware"
	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(database *sql.DB, cfg *config.Config, dockerClient *docker.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(middleware.CORS([]string{"*"}))

	rateLimiter := middleware.NewRateLimiter(500, 15*time.Minute)
	r.Use(rateLimiter.Middleware)

	authHandler := handlers.NewAuthHandler(database, cfg.JWTSecret)
	serverHandler := handlers.NewServerHandler(database, dockerClient, cfg)
	versionsHandler := handlers.NewVersionsHandler()
	javaHandler := handlers.NewJavaHandler()
	fileUploadHandler := handlers.NewFileUploadHandler(database, cfg)

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

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(database, cfg.JWTSecret))

			r.Route("/servers", func(r chi.Router) {
				r.Get("/", serverHandler.List)
				r.Post("/", serverHandler.Create)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", serverHandler.Get)
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
				})
			})
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	return r
}
