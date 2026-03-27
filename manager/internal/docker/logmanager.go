package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/demimine/manager/internal/spark"
	"github.com/demimine/manager/internal/websocket"
	"github.com/docker/docker/api/types/events"
)

var sparkURLRegex = regexp.MustCompile(`https://spark\.lucko\.me/[A-Za-z0-9]{8,}`)

type LogStream struct {
	serverID   int64
	container  string
	cancelFunc context.CancelFunc
}

type LogManager struct {
	docker              *Client
	db                  *sql.DB
	hub                 *websocket.Hub
	streams             map[int64]*LogStream
	proxyStreams        map[int64]*LogStream
	mu                  sync.RWMutex
	serverIDs           map[string]int64
	proxyIDs            map[string]int64
	pendingProfilerURLs map[int64]chan string
	urlMu               sync.RWMutex
}

func NewLogManager(dockerClient *Client, db *sql.DB, hub *websocket.Hub) *LogManager {
	return &LogManager{
		docker:              dockerClient,
		db:                  db,
		hub:                 hub,
		streams:             make(map[int64]*LogStream),
		proxyStreams:        make(map[int64]*LogStream),
		serverIDs:           make(map[string]int64),
		proxyIDs:            make(map[string]int64),
		pendingProfilerURLs: make(map[int64]chan string),
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

	var containerName = "demimine-" + SanitizeName(name)
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

	containerName = strings.TrimPrefix(containerName, "/")
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
	defer func() {
		lm.mu.Lock()
		delete(lm.streams, serverID)
		delete(lm.serverIDs, containerName)
		lm.mu.Unlock()
		log.Printf("Log stream ended for server %d (%s)", serverID, containerName)
	}()

	const maxRetries = 3
	var retryCount int

	for retryCount <= maxRetries {
		if retryCount > 0 {
			delay := time.Duration(1<<uint(retryCount-1)) * time.Second
			log.Printf("Retrying log stream for server %d in %v (attempt %d/%d)", serverID, delay, retryCount, maxRetries)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}

		logChan := make(chan string, 100)
		errChan := make(chan error, 1)
		streamClosed := false

		go func() {
			defer close(logChan)
			if err := lm.docker.StreamLogs(ctx, containerName, logChan); err != nil {
				errChan <- err
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case logLine, ok := <-logChan:
				if !ok {
					streamClosed = true
					break
				}
				if logLine != "" {
					if err := lm.BroadcastLog(serverID, logLine); err != nil {
						log.Printf("Failed to broadcast log for server %d: %v", serverID, err)
					}
					log.Printf("[%d] %s", serverID, logLine)
				}
			}

			if streamClosed {
				select {
				case err := <-errChan:
					if err != nil {
						log.Printf("Log stream error for server %d: %v", serverID, err)
						retryCount++
					} else {
						return
					}
				default:
					return
				}
				break
			}
		}
	}

	log.Printf("Max retries reached for server %d, giving up", serverID)
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
		log.Printf("Starting log streaming for running server %d (%s)", s.ID, s.Name)
		if err := lm.StartContainerStreamingByContainer(ctx, s.ID, s.Name, "demimine-"+SanitizeName(s.Name)); err != nil {
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
	normalizedContainerName := strings.TrimPrefix(containerName, "/")

	if strings.HasPrefix(normalizedContainerName, "demimine-proxy-") {
		lm.handleProxyContainerEvent(event, containerName)
		return
	}

	serverName := strings.TrimPrefix(normalizedContainerName, "demimine-")

	lm.mu.RLock()
	serverID, exists := lm.serverIDs[normalizedContainerName]
	lm.mu.RUnlock()

	if !exists {
		var id int64
		err := lm.db.QueryRow("SELECT id FROM servers WHERE name = ?", serverName).Scan(&id)
		if err == sql.ErrNoRows {
			err = lm.db.QueryRow("SELECT id FROM servers WHERE name = ? COLLATE NOCASE", serverName).Scan(&id)
		}
		if err != nil {
			log.Printf("Failed to find server ID for container %s (name=%s): %v", containerName, serverName, err)
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

func (lm *LogManager) handleProxyContainerEvent(event events.Message, containerName string) {
	normalizedContainerName := strings.TrimPrefix(containerName, "/")
	proxyName := strings.TrimPrefix(normalizedContainerName, "demimine-proxy-")

	lm.mu.RLock()
	proxyID, exists := lm.proxyIDs[normalizedContainerName]
	lm.mu.RUnlock()

	if !exists {
		var id int64
		err := lm.db.QueryRow("SELECT id FROM proxies WHERE name = ?", proxyName).Scan(&id)
		if err == sql.ErrNoRows {
			err = lm.db.QueryRow("SELECT id FROM proxies WHERE name = ? COLLATE NOCASE", proxyName).Scan(&id)
		}
		if err != nil {
			log.Printf("Failed to find proxy ID for container %s (name=%s): %v", containerName, proxyName, err)
			return
		}
		proxyID = id
	}

	switch event.Action {
	case "start":
		lm.StartProxyLogStreaming(context.Background(), proxyID, proxyName)
	case "die", "stop", "kill":
		lm.StopProxyLogStreaming(proxyID)
	}
}

func (lm *LogManager) StartProxyLogStreaming(ctx context.Context, proxyID int64, name string) error {
	if name == "" {
		var n string
		err := lm.db.QueryRow("SELECT name FROM proxies WHERE id = ?", proxyID).Scan(&n)
		if err != nil {
			return fmt.Errorf("failed to get proxy name: %w", err)
		}
		name = n
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, exists := lm.proxyStreams[proxyID]; exists {
		return fmt.Errorf("stream already exists for proxy %d", proxyID)
	}

	containerName := "demimine-proxy-" + SanitizeName(name)
	containerName = strings.TrimPrefix(containerName, "/")
	lm.proxyIDs[containerName] = proxyID

	streamCtx, cancel := context.WithCancel(ctx)

	lm.proxyStreams[proxyID] = &LogStream{
		serverID:   proxyID,
		container:  containerName,
		cancelFunc: cancel,
	}

	go lm.streamProxyLogs(streamCtx, proxyID, containerName)

	log.Printf("Started log streaming for proxy %d (%s)", proxyID, name)
	return nil
}

func (lm *LogManager) streamProxyLogs(ctx context.Context, proxyID int64, containerName string) {
	defer func() {
		lm.mu.Lock()
		delete(lm.proxyStreams, proxyID)
		delete(lm.proxyIDs, containerName)
		lm.mu.Unlock()
		log.Printf("Log stream ended for proxy %d (%s)", proxyID, containerName)
	}()

	const maxRetries = 3
	var retryCount int

	for retryCount <= maxRetries {
		if retryCount > 0 {
			delay := time.Duration(1<<uint(retryCount-1)) * time.Second
			log.Printf("Retrying log stream for proxy %d in %v (attempt %d/%d)", proxyID, delay, retryCount, maxRetries)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}

		logChan := make(chan string, 100)
		errChan := make(chan error, 1)
		streamClosed := false

		go func() {
			defer close(logChan)
			if err := lm.docker.StreamLogs(ctx, containerName, logChan); err != nil {
				errChan <- err
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case logLine, ok := <-logChan:
				if !ok {
					streamClosed = true
					break
				}
				if logLine != "" {
					if err := lm.BroadcastProxyLog(proxyID, logLine); err != nil {
						log.Printf("Failed to broadcast log for proxy %d: %v", proxyID, err)
					}
					log.Printf("[proxy:%d] %s", proxyID, logLine)
				}
			}

			if streamClosed {
				select {
				case err := <-errChan:
					if err != nil {
						log.Printf("Log stream error for proxy %d: %v", proxyID, err)
						retryCount++
					} else {
						return
					}
				default:
					return
				}
				break
			}
		}
	}

	log.Printf("Max retries reached for proxy %d, giving up", proxyID)
}

func (lm *LogManager) StopProxyLogStreaming(proxyID int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	stream, exists := lm.proxyStreams[proxyID]
	if !exists {
		return
	}

	stream.cancelFunc()
	delete(lm.proxyStreams, proxyID)

	var name string
	for containerName, id := range lm.proxyIDs {
		if id == proxyID {
			name = strings.TrimPrefix(containerName, "demimine-proxy-")
			delete(lm.proxyIDs, containerName)
			break
		}
	}

	log.Printf("Stopped log streaming for proxy %d (%s)", proxyID, name)
}

func (lm *LogManager) BroadcastProxyLog(proxyID int64, logLine string) error {
	msg := map[string]interface{}{
		"type":     "log",
		"proxy_id": proxyID,
		"log_line": logLine,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	lm.hub.Broadcast(data)

	return nil
}

func (lm *LogManager) StartStreamingForRunningProxies(ctx context.Context) error {
	type ProxyInfo struct {
		ID   int64
		Name string
	}

	var proxies []ProxyInfo
	rows, err := lm.db.Query("SELECT id, name FROM proxies WHERE status = 'running'")
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
		log.Printf("Starting log streaming for running proxy %d (%s)", p.ID, p.Name)
		if err := lm.StartProxyLogStreaming(ctx, p.ID, p.Name); err != nil {
			log.Printf("Failed to start log streaming for proxy %d: %v", p.ID, err)
		}
	}

	return nil
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

	if url := lm.extractProfilerURL(logLine); url != "" {
		lm.broadcastProfilerURL(serverID, url)
	}

	return nil
}

func (lm *LogManager) extractProfilerURL(logLine string) string {
	// Strip ANSI codes - Fabric/Forge/NeoForge may have colored output
	cleanLine := spark.StripColorCodes(logLine)

	// Match only spark report URLs (alphanumeric hash, not docs URLs)
	return sparkURLRegex.FindString(cleanLine)
}

func (lm *LogManager) broadcastProfilerURL(serverID int64, url string) {
	lm.urlMu.RLock()
	urlChan, exists := lm.pendingProfilerURLs[serverID]
	lm.urlMu.RUnlock()

	if exists && urlChan != nil {
		select {
		case urlChan <- url:
			log.Printf("[Spark] Profiler URL captured for server %d: %s", serverID, url)
		default:
			log.Printf("[Spark] Profiler URL channel closed for server %d", serverID)
		}
	}
}

func (lm *LogManager) getContainerNameByServerID(serverID int64) string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for containerName, id := range lm.serverIDs {
		if id == serverID {
			return containerName
		}
	}
	return ""
}

func (lm *LogManager) WaitForProfilerURL(serverID int64) <-chan string {
	lm.urlMu.Lock()
	defer lm.urlMu.Unlock()

	if _, exists := lm.pendingProfilerURLs[serverID]; exists {
		delete(lm.pendingProfilerURLs, serverID)
	}

	urlChan := make(chan string, 1)
	lm.pendingProfilerURLs[serverID] = urlChan
	return urlChan
}

func (lm *LogManager) CancelProfilerURLWait(serverID int64) {
	lm.urlMu.Lock()
	defer lm.urlMu.Unlock()

	if urlChan, exists := lm.pendingProfilerURLs[serverID]; exists {
		close(urlChan)
		delete(lm.pendingProfilerURLs, serverID)
	}
}
