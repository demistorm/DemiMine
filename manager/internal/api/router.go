package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/demimine/manager/internal/api/handlers"
	"github.com/demimine/manager/internal/api/middleware"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(database *sql.DB, jwtSecret string) *chi.Mux {
	r := chi.NewRouter()
	
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging)
	r.Use(middleware.CORS([]string{"*"}))
	
	rateLimiter := middleware.NewRateLimiter(500, 15*time.Minute)
	r.Use(rateLimiter.Middleware)
	
	authHandler := handlers.NewAuthHandler(database, jwtSecret)
	serverHandler := handlers.NewServerHandler(database)
	
	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/setup", authHandler.Setup)
			r.Post("/login", authHandler.Login)
			r.Post("/logout", authHandler.Logout)
			r.Get("/status", authHandler.Status)
		})
		
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(database, jwtSecret))
			
			r.Route("/servers", func(r chi.Router) {
				r.Get("/", serverHandler.List)
				r.Post("/", serverHandler.Create)
				r.Get("/{id}", serverHandler.Get)
				r.Delete("/{id}", serverHandler.Delete)
			})
		})
	})
	
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	
	return r
}
