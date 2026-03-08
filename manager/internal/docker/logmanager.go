package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/demimine/manager/internal/websocket"
	"github.com/docker/docker/api/types/events"
)

type LogStream struct {
	serverID   int64
	container  string
	cancelFunc context.CancelFunc
}

type LogManager struct {
	docker    *Client
	db        *sql.DB
	hub       *websocket.Hub
	streams   map[int64]*LogStream
	mu        sync.RWMutex
	serverIDs map[string]int64
}

func NewLogManager(dockerClient *Client, db *sql.DB, hub *websocket.Hub) *LogManager {
	return &LogManager{
		docker:    dockerClient,
		db:        db,
		hub:       hub,
		streams:   make(map[int64]*LogStream),
		serverIDs: make(map[string]int64),
	}
}

func (lm *LogManager) StartContainerStreaming(ctx context.Context, serverID int64, name string) error {
	if name == "" {
		var n string
		err := lm.db.QueryRow("SELECT name FROM servers WHERE id = ?", serverID).Scan(&n)
		if err != nil {
			return fmt.Errorf("failed to get server name: %w", err)
		}
		name = n
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, exists := lm.streams[serverID]; exists {
		return fmt.Errorf("stream already exists for server %d", serverID)
	}

	var containerName = "demimine-" + sanitizeName(name)
	lm.serverIDs[containerName] = serverID

	streamCtx, cancel := context.WithCancel(ctx)

	lm.streams[serverID] = &LogStream{
		serverID:   serverID,
		container:  containerName,
		cancelFunc: cancel,
	}

	go lm.streamLogs(streamCtx, serverID, containerName)

	log.Printf("Started log streaming for server %d (%s)", serverID, name)
	return nil
}

func (lm *LogManager) StartContainerStreamingByContainer(ctx context.Context, serverID int64, name, containerName string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, exists := lm.streams[serverID]; exists {
		return fmt.Errorf("stream already exists for server %d", serverID)
	}

	containerName = "demimine-" + sanitizeName(name)
	lm.serverIDs[containerName] = serverID

	streamCtx, cancel := context.WithCancel(ctx)

	lm.streams[serverID] = &LogStream{
		serverID:   serverID,
		container:  containerName,
		cancelFunc: cancel,
	}

	go lm.streamLogs(streamCtx, serverID, containerName)

	log.Printf("Started log streaming for server %d (%s)", serverID, name)
	return nil
}

func (lm *LogManager) streamLogs(ctx context.Context, serverID int64, containerName string) {
	logChan := make(chan string, 100)

	go func() {
		defer close(logChan)
		if err := lm.docker.StreamLogs(ctx, containerName, logChan); err != nil {
			log.Printf("Log stream error for server %d: %v", serverID, err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case logLine, ok := <-logChan:
			if !ok {
				return
			}
			if logLine != "" {
				if err := lm.BroadcastLog(serverID, logLine); err != nil {
					log.Printf("Failed to broadcast log for server %d: %v", serverID, err)
				}
				log.Printf("[%d] %s", serverID, logLine)
			}
		}
	}
}

func (lm *LogManager) StopContainerStreaming(serverID int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	stream, exists := lm.streams[serverID]
	if !exists {
		return
	}

	stream.cancelFunc()
	delete(lm.streams, serverID)

	var name string
	for containerName, id := range lm.serverIDs {
		if id == serverID {
			name = strings.TrimPrefix(containerName, "demimine-")
			delete(lm.serverIDs, containerName)
			break
		}
	}

	log.Printf("Stopped log streaming for server %d (%s)", serverID, name)
}

func (lm *LogManager) StartStreamingForRunningContainers(ctx context.Context, db *sql.DB) error {
	type ServerInfo struct {
		ID   int64
		Name string
	}

	var servers []ServerInfo
	rows, err := db.Query("SELECT id, name FROM servers WHERE status = 'running'")
	if err != nil {
		return fmt.Errorf("failed to query running servers: %w", err)
	}

	for rows.Next() {
		var s ServerInfo
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			log.Printf("Failed to scan server: %v", err)
			continue
		}
		servers = append(servers, s)
	}
	rows.Close()

	for _, s := range servers {
		lm.mu.RLock()
		_, exists := lm.streams[s.ID]
		lm.mu.RUnlock()

		if exists {
			log.Printf("Already streaming logs for server %d (%s)", s.ID, s.Name)
			continue
		}

		log.Printf("Starting log streaming for running server %d (%s)", s.ID, s.Name)
		if err := lm.StartContainerStreamingByContainer(ctx, s.ID, s.Name, "demimine-"+sanitizeName(s.Name)); err != nil {
			log.Printf("Failed to start log streaming for server %d: %v", s.ID, err)
		}
	}

	return nil
}

func (lm *LogManager) HandleContainerEvent(event events.Message) {
	if event.Actor.ID == "" {
		return
	}

	containerName := event.Actor.Attributes["name"]
	serverName := strings.TrimPrefix(containerName, "demimine-")

	lm.mu.RLock()
	serverID, exists := lm.serverIDs[event.Actor.ID]
	lm.mu.RUnlock()

	if !exists {
		var id int64
		err := lm.db.QueryRow("SELECT id FROM servers WHERE name = ?", serverName).Scan(&id)
		if err != nil {
			return
		}

		serverID = id
	}

	switch event.Action {
	case "start":
		lm.StartContainerStreamingByContainer(context.Background(), serverID, serverName, containerName)
	case "die", "stop", "kill":
		lm.StopContainerStreaming(serverID)
	}
}

func (lm *LogManager) BroadcastLog(serverID int64, logLine string) error {
	msg := map[string]interface{}{
		"type":      "log",
		"server_id": serverID,
		"log_line":  logLine,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	lm.hub.Broadcast(data)

	return nil
}
