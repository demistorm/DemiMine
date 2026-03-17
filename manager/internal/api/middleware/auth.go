package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/demimine/manager/pkg/auth"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func Auth(db *sql.DB, jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie("session"); err == nil {
				claims, err := auth.ValidateToken(cookie.Value, jwtSecret)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				key := strings.TrimPrefix(authHeader, "Bearer ")

				claims, err := auth.ValidateToken(key, jwtSecret)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				var keyHash string
				err = db.QueryRow(
					"SELECT key_hash FROM api_keys WHERE key_hash = ?",
					HashAPIKey(key),
				).Scan(&keyHash)

				if err == nil {
					_, _ = db.Exec(
						"UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key_hash = ?",
						keyHash,
					)
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "unauthorized",
			})
		})
	}
}

func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return base64.StdEncoding.EncodeToString(h[:])
}

func GetUserID(r *http.Request) int {
	if userID, ok := r.Context().Value(UserIDKey).(int); ok {
		return userID
	}
	return 0
}
