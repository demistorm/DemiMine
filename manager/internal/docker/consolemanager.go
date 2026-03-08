package docker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

type ConsoleStream struct {
	serverID   int64
	container  string
	conn       types.HijackedResponse
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

type ConsoleManager struct {
	docker    *Client
	db        *sql.DB
	consoles  map[int64]*ConsoleStream
	mu        sync.RWMutex
	serverIDs map[string]int64
}

func NewConsoleManager(dockerClient *Client, db *sql.DB) *ConsoleManager {
	return &ConsoleManager{
		docker:    dockerClient,
		db:        db,
		consoles:  make(map[int64]*ConsoleStream),
		serverIDs: make(map[string]int64),
	}
}

func (cm *ConsoleManager) StartConsoleStreaming(ctx context.Context, serverID int64, name string) error {
	if name == "" {
		var n string
		err := cm.db.QueryRow("SELECT name FROM servers WHERE id = ?", serverID).Scan(&n)
		if err != nil {
			return fmt.Errorf("failed to get server name: %w", err)
		}
		name = n
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.consoles[serverID]; exists {
		return fmt.Errorf("console already exists for server %d", serverID)
	}

	containerName := "demimine-" + sanitizeName(name)
	cm.serverIDs[containerName] = serverID

	streamCtx, cancel := context.WithCancel(ctx)

	cfg := container.AttachOptions{
		Stdin:  true,
		Stdout: false,
		Stderr: false,
		Stream: true,
	}

	conn, err := cm.docker.cli.ContainerAttach(ctx, containerName, cfg)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to attach to container stdin: %w", err)
	}

	cm.consoles[serverID] = &ConsoleStream{
		serverID:   serverID,
		container:  containerName,
		conn:       conn,
		cancelFunc: cancel,
	}

	go func() {
		<-streamCtx.Done()
		cm.mu.Lock()
		defer cm.mu.Unlock()
		if stream, ok := cm.consoles[serverID]; ok {
			stream.conn.Close()
			delete(cm.consoles, serverID)
		}
	}()

	log.Printf("Started console streaming for server %d (%s)", serverID, name)
	return nil
}

func (cm *ConsoleManager) StopConsoleStreaming(serverID int64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	stream, exists := cm.consoles[serverID]
	if !exists {
		return
	}

	stream.cancelFunc()
	stream.conn.Close()
	delete(cm.consoles, serverID)

	var name string
	for containerName, id := range cm.serverIDs {
		if id == serverID {
			name = strings.TrimPrefix(containerName, "demimine-")
			delete(cm.serverIDs, containerName)
			break
		}
	}

	log.Printf("Stopped console streaming for server %d (%s)", serverID, name)
}

func (cm *ConsoleManager) SendCommand(serverID int64, command string) error {
	cm.mu.RLock()
	stream, exists := cm.consoles[serverID]
	cm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no console connection for server %d", serverID)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	_, err := fmt.Fprintln(stream.conn.Conn, command)
	if err != nil {
		return fmt.Errorf("failed to write command to stdin: %w", err)
	}

	log.Printf("Sent command to server %d (%s): %s", serverID, stream.container, command)
	return nil
}

func (cm *ConsoleManager) HandleContainerEvent(containerName string, action string) {
	if containerName == "" {
		return
	}

	cm.mu.RLock()
	serverID, exists := cm.serverIDs[containerName]
	cm.mu.RUnlock()

	if !exists {
		serverName := strings.TrimPrefix(containerName, "demimine-")
		var id int64
		err := cm.db.QueryRow("SELECT id FROM servers WHERE name = ?", serverName).Scan(&id)
		if err != nil {
			return
		}
		serverID = id
	}

	switch action {
	case "start":
		cm.StartConsoleStreaming(context.Background(), serverID, "")
	case "die", "stop", "kill":
		cm.StopConsoleStreaming(serverID)
	}
}

func (cm *ConsoleManager) StartConsolesForRunningContainers(ctx context.Context) error {
	type ServerInfo struct {
		ID   int64
		Name string
	}

	var servers []ServerInfo
	rows, err := cm.db.Query("SELECT id, name FROM servers WHERE status = 'running'")
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
		cm.mu.RLock()
		_, exists := cm.consoles[s.ID]
		cm.mu.RUnlock()

		if exists {
			log.Printf("Already streaming console for server %d (%s)", s.ID, s.Name)
			continue
		}

		log.Printf("Starting console streaming for running server %d (%s)", s.ID, s.Name)
		if err := cm.StartConsoleStreaming(ctx, s.ID, s.Name); err != nil {
			log.Printf("Failed to start console streaming for server %d: %v", s.ID, err)
		}
	}

	return nil
}

func (cm *ConsoleManager) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, stream := range cm.consoles {
		stream.cancelFunc()
		stream.conn.Close()
	}

	cm.consoles = make(map[int64]*ConsoleStream)
	cm.serverIDs = make(map[string]int64)
	return nil
}
