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
	docker        *Client
	db            *sql.DB
	consoles      map[int64]*ConsoleStream
	proxyConsoles map[int64]*ConsoleStream
	mu            sync.RWMutex
	serverIDs     map[string]int64
	proxyIDs      map[string]int64
}

func NewConsoleManager(dockerClient *Client, db *sql.DB) *ConsoleManager {
	return &ConsoleManager{
		docker:        dockerClient,
		db:            db,
		consoles:      make(map[int64]*ConsoleStream),
		proxyConsoles: make(map[int64]*ConsoleStream),
		serverIDs:     make(map[string]int64),
		proxyIDs:      make(map[string]int64),
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

	containerName := "demimine-" + SanitizeName(name)
	containerName = strings.TrimPrefix(containerName, "/")
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

	normalizedContainerName := strings.TrimPrefix(containerName, "/")

	if strings.HasPrefix(normalizedContainerName, "demimine-proxy-") {
		cm.HandleProxyContainerEvent(containerName, action)
		return
	}

	cm.mu.RLock()
	serverID, exists := cm.serverIDs[normalizedContainerName]
	cm.mu.RUnlock()

	if !exists {
		serverName := strings.TrimPrefix(normalizedContainerName, "demimine-")
		var id int64
		err := cm.db.QueryRow("SELECT id FROM servers WHERE name = ?", serverName).Scan(&id)
		if err == sql.ErrNoRows {
			err = cm.db.QueryRow("SELECT id FROM servers WHERE name = ? COLLATE NOCASE", serverName).Scan(&id)
		}
		if err != nil {
			log.Printf("Failed to find server ID for container %s (name=%s): %v", containerName, serverName, err)
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
	defer rows.Close()

	for rows.Next() {
		var s ServerInfo
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			log.Printf("Failed to scan server: %v", err)
			continue
		}
		servers = append(servers, s)
	}

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

	if err := cm.StartConsolesForRunningProxies(ctx); err != nil {
		log.Printf("Failed to start consoles for running proxies: %v", err)
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

	for _, stream := range cm.proxyConsoles {
		stream.cancelFunc()
		stream.conn.Close()
	}

	cm.consoles = make(map[int64]*ConsoleStream)
	cm.proxyConsoles = make(map[int64]*ConsoleStream)
	cm.serverIDs = make(map[string]int64)
	cm.proxyIDs = make(map[string]int64)
	return nil
}

func (cm *ConsoleManager) StartProxyConsoleStreaming(ctx context.Context, proxyID int64, name string) error {
	if name == "" {
		var n string
		err := cm.db.QueryRow("SELECT name FROM proxies WHERE id = ?", proxyID).Scan(&n)
		if err != nil {
			return fmt.Errorf("failed to get proxy name: %w", err)
		}
		name = n
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.proxyConsoles[proxyID]; exists {
		return fmt.Errorf("console already exists for proxy %d", proxyID)
	}

	containerName := "demimine-proxy-" + SanitizeName(name)
	containerName = strings.TrimPrefix(containerName, "/")
	cm.proxyIDs[containerName] = proxyID

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
		return fmt.Errorf("failed to attach to proxy container stdin: %w", err)
	}

	cm.proxyConsoles[proxyID] = &ConsoleStream{
		serverID:   proxyID,
		container:  containerName,
		conn:       conn,
		cancelFunc: cancel,
	}

	go func() {
		<-streamCtx.Done()
		cm.mu.Lock()
		defer cm.mu.Unlock()
		if stream, ok := cm.proxyConsoles[proxyID]; ok {
			stream.conn.Close()
			delete(cm.proxyConsoles, proxyID)
		}
	}()

	log.Printf("Started console streaming for proxy %d (%s)", proxyID, name)
	return nil
}

func (cm *ConsoleManager) StopProxyConsoleStreaming(proxyID int64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	stream, exists := cm.proxyConsoles[proxyID]
	if !exists {
		return
	}

	stream.cancelFunc()
	stream.conn.Close()
	delete(cm.proxyConsoles, proxyID)

	var name string
	for containerName, id := range cm.proxyIDs {
		if id == proxyID {
			name = strings.TrimPrefix(containerName, "demimine-proxy-")
			delete(cm.proxyIDs, containerName)
			break
		}
	}

	log.Printf("Stopped console streaming for proxy %d (%s)", proxyID, name)
}

func (cm *ConsoleManager) SendProxyCommand(proxyID int64, command string) error {
	cm.mu.RLock()
	stream, exists := cm.proxyConsoles[proxyID]
	cm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no console connection for proxy %d", proxyID)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	_, err := fmt.Fprintln(stream.conn.Conn, command)
	if err != nil {
		return fmt.Errorf("failed to write command to proxy stdin: %w", err)
	}

	log.Printf("Sent command to proxy %d (%s): %s", proxyID, stream.container, command)
	return nil
}

func (cm *ConsoleManager) HandleProxyContainerEvent(containerName string, action string) {
	if containerName == "" {
		return
	}

	normalizedContainerName := strings.TrimPrefix(containerName, "/")

	cm.mu.RLock()
	proxyID, exists := cm.proxyIDs[normalizedContainerName]
	cm.mu.RUnlock()

	if !exists {
		proxyName := strings.TrimPrefix(normalizedContainerName, "demimine-proxy-")
		var id int64
		err := cm.db.QueryRow("SELECT id FROM proxies WHERE name = ?", proxyName).Scan(&id)
		if err == sql.ErrNoRows {
			err = cm.db.QueryRow("SELECT id FROM proxies WHERE name = ? COLLATE NOCASE", proxyName).Scan(&id)
		}
		if err != nil {
			log.Printf("Failed to find proxy ID for container %s (name=%s): %v", containerName, proxyName, err)
			return
		}
		proxyID = id
	}

	switch action {
	case "start":
		cm.StartProxyConsoleStreaming(context.Background(), proxyID, "")
	case "die", "stop", "kill":
		cm.StopProxyConsoleStreaming(proxyID)
	}
}

func (cm *ConsoleManager) StartConsolesForRunningProxies(ctx context.Context) error {
	type ProxyInfo struct {
		ID   int64
		Name string
	}

	var proxies []ProxyInfo
	rows, err := cm.db.Query("SELECT id, name FROM proxies WHERE status = 'running'")
	if err != nil {
		return fmt.Errorf("failed to query running proxies: %w", err)
	}

	for rows.Next() {
		var p ProxyInfo
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			log.Printf("Failed to scan proxy: %v", err)
			continue
		}
		proxies = append(proxies, p)
	}
	rows.Close()

	for _, p := range proxies {
		cm.mu.RLock()
		_, exists := cm.proxyConsoles[p.ID]
		cm.mu.RUnlock()

		if exists {
			log.Printf("Already streaming console for proxy %d (%s)", p.ID, p.Name)
			continue
		}

		log.Printf("Starting console streaming for running proxy %d (%s)", p.ID, p.Name)
		if err := cm.StartProxyConsoleStreaming(ctx, p.ID, p.Name); err != nil {
			log.Printf("Failed to start console streaming for proxy %d: %v", p.ID, err)
		}
	}

	return nil
}
