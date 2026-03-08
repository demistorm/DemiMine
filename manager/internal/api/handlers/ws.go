package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/websocket"
	"github.com/golang-jwt/jwt/v5"
	gws "github.com/gorilla/websocket"
)

type WSHandler struct {
	hub            *websocket.Hub
	db             *sql.DB
	docker         *docker.Client
	consoleManager *docker.ConsoleManager
	cfg            *config.Config
}

func NewWSHandler(db *sql.DB, docker *docker.Client, consoleManager *docker.ConsoleManager, cfg *config.Config) *WSHandler {
	hub := websocket.NewHub()
	return &WSHandler{
		db:             db,
		docker:         docker,
		consoleManager: consoleManager,
		cfg:            cfg,
		hub:            hub,
	}
}

func (h *WSHandler) Run() {
	go h.hub.Run()
}

func (h *WSHandler) GetHub() *websocket.Hub {
	return h.hub
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	log.Printf("WebSocket connection attempt from %s", r.RemoteAddr)

	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		if token != "" {
			token = strings.TrimPrefix(token, "Bearer ")
		}
	}

	if token == "" {
		log.Printf("WebSocket rejected: no token provided")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil {
		log.Printf("WebSocket rejected: invalid token: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
		return
	}

	upgrader := gws.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	log.Printf("WebSocket connection established from %s", r.RemoteAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := websocket.NewClient(h.hub, conn)
	h.hub.RegisterClient(client)

	log.Printf("WebSocket client registered, total clients: %d", h.hub.ClientCount())

	client.SetMessageHandler(func(msg websocket.Message) {
		log.Printf("Received WebSocket message: type=%s, server_id=%v, channel=%v, command=%v",
			msg.Type, msg.ServerID, msg.Channel, msg.Command)

		switch msg.Type {
		case websocket.MessageTypeSubscribe:
			if msg.Channel != nil {
				log.Printf("Client subscribing to channel: %s", *msg.Channel)
				client.Subscribe(*msg.Channel)
			}
		case websocket.MessageTypeUnsubscribe:
			if msg.Channel != nil {
				log.Printf("Client unsubscribing from channel: %s", *msg.Channel)
				client.Unsubscribe(*msg.Channel)
			}
		case websocket.MessageTypeCommand:
			if msg.ServerID != nil && msg.Command != nil {
				log.Printf("Queueing command for server %d: %s", *msg.ServerID, *msg.Command)
				go h.handleCommand(ctx, client, *msg.ServerID, *msg.Command)
			}
		}
	})

	go client.RunReadPump(ctx)
	go client.RunWritePump(ctx)

	<-ctx.Done()
}

func (h *WSHandler) handleCommand(ctx context.Context, client *websocket.Client, serverID int64, command string) {
	log.Printf("handleCommand: Processing command for server ID %d: %s", serverID, command)

	var name string
	err := h.db.QueryRow("SELECT name FROM servers WHERE id = ?", serverID).Scan(&name)
	if err == sql.ErrNoRows {
		log.Printf("handleCommand: Server with ID %d not found in database", serverID)
		client.SendError("server not found")
		return
	}
	if err != nil {
		log.Printf("handleCommand: Database error looking up server %d: %v", serverID, err)
		client.SendError("database error")
		return
	}

	log.Printf("handleCommand: Server found - ID: %d, Name: %s, Command: %s", serverID, name, command)

	if err := h.consoleManager.SendCommand(serverID, command); err != nil {
		log.Printf("handleCommand: Failed to send command to server %s: %v", name, err)
		client.SendError("failed to send command")
		return
	}

	log.Printf("handleCommand: Command successfully sent to server %s", name)

	result, err := h.db.Exec("INSERT INTO command_history (server_id, command) VALUES (?, ?)", serverID, command)
	if err != nil {
		log.Printf("handleCommand: Failed to insert command history: %v", err)
	} else {
		rowsAffected, _ := result.RowsAffected()
		log.Printf("handleCommand: Command history inserted, rows affected: %d", rowsAffected)
	}
}
