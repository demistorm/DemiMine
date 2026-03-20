package websocket

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type MessageType string

const (
	MessageTypeSubscribe    MessageType = "subscribe"
	MessageTypeUnsubscribe  MessageType = "unsubscribe"
	MessageTypeCommand      MessageType = "command"
	MessageTypeProxyCommand MessageType = "proxy_command"
	MessageTypeServerStatus MessageType = "server_status"
	MessageTypePlayerJoin   MessageType = "player_join"
	MessageTypePlayerLeave  MessageType = "player_leave"
	MessageTypeLog          MessageType = "log"
	MessageTypeResources    MessageType = "resources"
	MessageTypeCrash        MessageType = "crash"
	MessageTypeBackupStatus MessageType = "backup_status"
)

type Message struct {
	Type       MessageType `json:"type"`
	Channel    *string     `json:"channel,omitempty"`
	ServerID   *int64      `json:"server_id,omitempty"`
	ProxyID    *int64      `json:"proxy_id,omitempty"`
	Command    *string     `json:"command,omitempty"`
	Status     *string     `json:"status,omitempty"`
	PlayerName *string     `json:"player_name,omitempty"`
	PlayerUUID *string     `json:"player_uuid,omitempty"`
	LogLine    *string     `json:"log_line,omitempty"`
	CPUPercent *float64    `json:"cpu_percent,omitempty"`
	MemoryMB   *int64      `json:"memory_mb,omitempty"`
	CrashLogID *int64      `json:"crash_log_id,omitempty"`
}

type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	serverID  *int64
	channels  map[string]bool
	mu        sync.Mutex
	onMessage func(msg Message)
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		channels: make(map[string]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					h.mu.RUnlock()
					close(client.send)
					h.mu.Lock()
					delete(h.clients, client)
					h.mu.RUnlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}

func (h *Hub) Broadcast(message []byte) {
	h.broadcast <- message
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (c *Client) isSubscribedToServer(serverID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.channels["servers"]
}

func (c *Client) handleMessage(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch msg.Type {
	case MessageTypeSubscribe:
		if msg.Channel != nil {
			c.channels[*msg.Channel] = true
		}
	case MessageTypeUnsubscribe:
		if msg.Channel != nil {
			delete(c.channels, *msg.Channel)
		}
	}
}

func (c *Client) Subscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.channels[channel] = true
}

func (c *Client) Unsubscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.channels, channel)
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) ReadMessage() (int, []byte, error) {
	return c.conn.ReadMessage()
}

func (c *Client) WriteMessage(messageType int, data []byte) error {
	return c.conn.WriteMessage(messageType, data)
}

func (c *Client) GetSend() chan []byte {
	return c.send
}

func (c *Client) SetMessageHandler(handler func(msg Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = handler
}

func (c *Client) SendError(error string) {
	msg := map[string]string{
		"type":  "error",
		"error": error,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *Client) RunReadPump(ctx context.Context) {
	defer func() {
		c.hub.UnregisterClient(c)
		c.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			var msg Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			c.handleMessage(msg)

			if c.onMessage != nil {
				c.onMessage(msg)
			}
		}
	}
}

func (c *Client) RunWritePump(ctx context.Context) {
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.mu.Lock()
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			c.mu.Unlock()

			if err != nil {
				log.Printf("Failed to write message: %v", err)
				return
			}
		}
	}
}
