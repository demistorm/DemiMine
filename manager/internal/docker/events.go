package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"

	"github.com/demimine/manager/internal/websocket"
	"github.com/docker/docker/api/types/events"
)

type EventManager struct {
	docker         *Client
	db             *sql.DB
	hub            *websocket.Hub
	logManager     *LogManager
	consoleManager *ConsoleManager
	eventChan      chan events.Message
}

func NewEventManager(dockerClient *Client, db *sql.DB, hub *websocket.Hub, logManager *LogManager, consoleManager *ConsoleManager) *EventManager {
	return &EventManager{
		docker:         dockerClient,
		db:             db,
		hub:            hub,
		logManager:     logManager,
		consoleManager: consoleManager,
		eventChan:      make(chan events.Message, 100),
	}
}

func (em *EventManager) SetHub(hub *websocket.Hub) {
	em.hub = hub
}

func (em *EventManager) Start(ctx context.Context) {
	go em.watchContainerEvents(ctx)
}

func (em *EventManager) watchContainerEvents(ctx context.Context) {
	dockerEvents := make(chan events.Message, 100)

	go func() {
		if err := em.docker.ContainerEvents(ctx, dockerEvents); err != nil {
			log.Printf("Failed to watch container events: %v", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-dockerEvents:
			em.processEvent(event)
		}
	}
}

func (em *EventManager) processEvent(event events.Message) {
	log.Printf("Container event: %s %s", event.Action, event.Actor.ID)

	containerName := event.Actor.Attributes["name"]
	if containerName == "" {
		return
	}

	if event.Actor.Attributes["demimine.managed"] != "true" {
		return
	}

	em.logManager.HandleContainerEvent(event)
	em.consoleManager.HandleContainerEvent(containerName, string(event.Action))

	var serverID int64
	err := em.db.QueryRow("SELECT id FROM servers WHERE name = ?", event.Actor.Attributes["demimine.server_id"]).Scan(&serverID)
	if err != nil {
		log.Printf("Failed to find server ID for container %s: %v", containerName, err)
		return
	}

	em.broadcastStatusChange(serverID, event.Action)
}

func (em *EventManager) broadcastStatusChange(serverID int64, action events.Action) {
	var status string
	switch action {
	case "start":
		status = "running"
	case "die", "stop", "kill":
		status = "stopped"
	case "pause":
		status = "paused"
	case "unpause":
		status = "running"
	default:
		return
	}

	msg := map[string]interface{}{
		"type":      "server_status",
		"server_id": serverID,
		"status":    status,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal status message: %v", err)
		return
	}

	em.hub.Broadcast(data)
}
