package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"github.com/demimine/manager/internal/java"
	"github.com/demimine/manager/internal/util"
)

func getTZ() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	return "America/Chicago"
}

type ServerContainerConfig struct {
	Name         string
	ServerType   string
	Version      string
	RAMMB        int
	JVMFlags     string
	JavaOverride string
	ServerPath   string
	NetworkName  string
	HostPort     int
	UDPPort      int
}

func (c *Client) CreateServerContainer(ctx context.Context, cfg ServerContainerConfig) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	if c.cli == nil {
		return "", fmt.Errorf("docker client is nil")
	}

	containerName := "demimine-" + SanitizeName(cfg.Name)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in CreateServerContainer: %v", r)
		}
	}()

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
	} else if existing.ID != "" {
		_ = c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
			Force: true,
		})
	}

	javaVersion := java.GetRequiredJavaVersionForServerType(cfg.ServerType, cfg.Version)
	if cfg.JavaOverride != "" {
		javaVersion = cfg.JavaOverride
	}
	javaImage := "eclipse-temurin:21-jre-noble"
	switch javaVersion {
	case "8":
		javaImage = "eclipse-temurin:8-jre-noble"
	case "17":
		javaImage = "eclipse-temurin:17-jre-noble"
	case "25":
		javaImage = "eclipse-temurin:25-jre-noble"
	}

	_, _, err = c.cli.ImageInspectWithRaw(ctx, javaImage)
	if err != nil {
		reader, err := c.cli.ImagePull(ctx, javaImage, image.PullOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to pull image %s: %w", javaImage, err)
		}
		io.Copy(io.Discard, reader)
		reader.Close()
	}

	env := []string{
		"TERM=xterm",
		fmt.Sprintf("TZ=%s", getTZ()),
		fmt.Sprintf("SERVER_TYPE=%s", cfg.ServerType),
		fmt.Sprintf("MC_VERSION=%s", cfg.Version),
		fmt.Sprintf("RAM_MB=%d", cfg.RAMMB),
		fmt.Sprintf("JVM_FLAGS=%s", cfg.JVMFlags),
		fmt.Sprintf("RCON_PASSWORD=%s", os.Getenv("RCON_PASSWORD")),
	}

	cmd := []string{"sh", "/server/start.sh"}

	config := &container.Config{
		Image:        javaImage,
		Hostname:     containerName,
		Cmd:          cmd,
		Env:          env,
		WorkingDir:   "/server",
		ExposedPorts: nat.PortSet{"25565/tcp": {}, "39521/tcp": {}},
		Labels: map[string]string{
			"demimine.managed":   "true",
			"demimine.server_id": cfg.Name,
			"demimine.type":      "server",
		},
		Tty:       true,
		OpenStdin: true,
	}

	hostConfig := &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:/server:rw", cfg.ServerPath)},
		Resources: container.Resources{
			Memory: int64(cfg.RAMMB+512) * 1024 * 1024,
		},
		AutoRemove: true,
		RestartPolicy: container.RestartPolicy{
			Name: "no",
		},
	}

	if cfg.HostPort > 0 {
		containerPort := nat.Port(fmt.Sprintf("%d/tcp", cfg.HostPort))
		hostConfig.PortBindings = nat.PortMap{
			containerPort: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.HostPort)},
			},
		}
	}

	if cfg.UDPPort > 0 {
		udpPort := nat.Port(fmt.Sprintf("%d/udp", cfg.UDPPort))
		config.ExposedPorts[udpPort] = struct{}{}
		if hostConfig.PortBindings == nil {
			hostConfig.PortBindings = nat.PortMap{}
		}
		hostConfig.PortBindings[udpPort] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.UDPPort)},
		}
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			cfg.NetworkName: {},
		},
	}

	created, err := c.cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return created.ID, nil
}

func (c *Client) StartContainer(ctx context.Context, name string, cfg *ServerContainerConfig) error {
	containerName := "demimine-" + SanitizeName(name)

	exists, err := c.ContainerExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	if !exists {
		if cfg == nil {
			return fmt.Errorf("container %s does not exist and no config provided", containerName)
		}
		_, err = c.CreateServerContainer(ctx, *cfg)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}
	}

	return c.cli.ContainerStart(ctx, containerName, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, name string, timeout *int) error {
	if timeout == nil {
		t := 30
		timeout = &t
	}
	containerName := "demimine-" + SanitizeName(name)
	return c.cli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: timeout})
}

func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

func (c *Client) GetContainerStatus(ctx context.Context, containerName string) (string, error) {
	containerName = "demimine-" + SanitizeName(containerName)
	ctr, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "stopped", nil
	}

	switch ctr.State.Status {
	case "running":
		return "running", nil
	case "created", "paused":
		return "starting", nil
	case "exited":
		if ctr.State.ExitCode != 0 {
			return "crashed", nil
		}
		return "stopped", nil
	default:
		return ctr.State.Status, nil
	}
}

func (c *Client) GetContainerLogs(ctx context.Context, containerName string, tail int) ([]string, error) {
	containerName = "demimine-" + SanitizeName(containerName)

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
		Timestamps: false,
	}

	reader, err := c.cli.ContainerLogs(ctx, containerName, options)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Containers are created with Tty: true, so logs are NOT multiplexed
	// Use io.ReadAll instead of stdcopy.StdCopy
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs: %w", err)
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			lines = append(lines, strings.TrimSuffix(line, "\r"))
		}
	}

	return lines, nil
}

func (c *Client) ListManagedContainers(ctx context.Context) ([]types.Container, error) {
	filter := filters.NewArgs()
	filter.Add("label", "demimine.managed=true")

	return c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filter,
	})
}

func (c *Client) ContainerExists(ctx context.Context, name string) (bool, error) {
	name = "demimine-" + SanitizeName(name)
	_, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Client) GetContainerID(ctx context.Context, name string) (string, error) {
	name = "demimine-" + SanitizeName(name)
	ctr, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", err
	}
	return ctr.ID, nil
}

func SanitizeName(name string) string {
	return util.SanitizeName(name)
}

func (c *Client) CreateServerDirectory(basePath, serverName string) error {
	serverPath := filepath.Join(basePath, serverName)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}
	return nil
}

func (c *Client) WaitForContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	statusCh, errCh := c.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case <-statusCh:
	case err := <-errCh:
		if err != nil && err != io.EOF {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *Client) SyncServerStatus(ctx context.Context, database *sql.DB) {
	rows, err := database.Query("SELECT id, sanitized_name FROM servers WHERE status = 'running'")
	if err != nil {
		log.Printf("Failed to query running servers for status sync: %v", err)
		return
	}
	defer rows.Close()

	type serverInfo struct {
		id   int64
		name string
	}
	var servers []serverInfo

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Printf("Failed to scan server row: %v", err)
			continue
		}
		servers = append(servers, serverInfo{id: id, name: name})
	}

	for _, s := range servers {
		exists, _ := c.ContainerExists(ctx, s.name)
		if !exists {
			_, err := database.Exec("UPDATE servers SET status = 'stopped' WHERE id = ?", s.id)
			if err != nil {
				log.Printf("Failed to update server %d status: %v", s.id, err)
			} else {
				log.Printf("Synced server %d (%s): running -> stopped (container gone)", s.id, s.name)
			}
		}
	}
}

func (c *Client) GetContainerMemoryUsage(ctx context.Context, containerName string) (int64, error) {
	containerName = strings.TrimPrefix(containerName, "/")

	if !strings.HasPrefix(containerName, "demimine-") {
		containerName = "demimine-" + SanitizeName(containerName)
	}

	stats, err := c.cli.ContainerStats(ctx, containerName, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	var statsData map[string]interface{}
	if err := json.NewDecoder(stats.Body).Decode(&statsData); err != nil {
		return 0, fmt.Errorf("failed to decode container stats: %w", err)
	}

	memoryStats, ok := statsData["memory_stats"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("memory_stats not found in container stats")
	}

	usage, ok := memoryStats["usage"].(float64)
	if !ok {
		return 0, fmt.Errorf("usage not found in memory_stats")
	}

	usageMB := int64(usage) / 1024 / 1024
	return usageMB, nil
}

func (c *Client) GetTotalDemimineMemoryUsage(ctx context.Context) (int64, error) {
	containers, err := c.ListManagedContainers(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list containers: %w", err)
	}

	var totalUsage int64
	for _, ctr := range containers {
		stats, err := c.cli.ContainerStats(ctx, ctr.ID, false)
		if err != nil {
			continue
		}

		var statsData map[string]interface{}
		if err := json.NewDecoder(stats.Body).Decode(&statsData); err != nil {
			stats.Body.Close()
			continue
		}
		stats.Body.Close()

		memoryStats, ok := statsData["memory_stats"].(map[string]interface{})
		if !ok {
			continue
		}

		usage, ok := memoryStats["usage"].(float64)
		if !ok {
			continue
		}

		totalUsage += int64(usage) / 1024 / 1024
	}

	return totalUsage, nil
}

func (c *Client) WaitForContainerRemoval(ctx context.Context, containerName string, timeout time.Duration) error {
	containerName = "demimine-" + SanitizeName(containerName)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for container %s to be removed", containerName)
		case <-ticker.C:
			_, err := c.cli.ContainerInspect(ctx, containerName)
			if err != nil {
				if strings.Contains(err.Error(), "No such container") {
					return nil
				}
			}
		}
	}
}
