package docker

import (
	"context"
	"database/sql"
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
)

type ServerContainerConfig struct {
	Name        string
	ServerType  string
	Version     string
	RAMMB       int
	JavaPath    string
	ServerPath  string
	NetworkName string
	HostPort    int
}

func (c *Client) CreateServerContainer(ctx context.Context, cfg ServerContainerConfig) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	if c.cli == nil {
		return "", fmt.Errorf("docker client is nil")
	}

	containerName := "demimine-" + sanitizeName(cfg.Name)

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

	javaVersion := java.GetRequiredJavaVersion(cfg.Version)
	javaImage := "eclipse-temurin:21-jre-alpine"
	switch javaVersion {
	case "8":
		javaImage = "eclipse-temurin:8-jre-alpine"
	case "17":
		javaImage = "eclipse-temurin:17-jre-alpine"
	case "25":
		javaImage = "eclipse-temurin:25-jre-ubi10-minimal"
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
		fmt.Sprintf("SERVER_TYPE=%s", cfg.ServerType),
		fmt.Sprintf("MC_VERSION=%s", cfg.Version),
		fmt.Sprintf("RAM_MB=%d", cfg.RAMMB),
	}

	cmd := []string{"sh", "/server/start.sh"}

	config := &container.Config{
		Image:        javaImage,
		Hostname:     containerName,
		Cmd:          cmd,
		Env:          env,
		WorkingDir:   "/server",
		ExposedPorts: nat.PortSet{"25565/tcp": {}},
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
			Memory: int64(cfg.RAMMB) * 1024 * 1024,
		},
		AutoRemove: true,
		RestartPolicy: container.RestartPolicy{
			Name: "no",
		},
	}

	if cfg.HostPort > 0 {
		hostConfig.PortBindings = nat.PortMap{
			"25565/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.HostPort)},
			},
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
	containerName := "demimine-" + sanitizeName(name)

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
	containerName := "demimine-" + sanitizeName(name)
	return c.cli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: timeout})
}

func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

func (c *Client) GetContainerStatus(ctx context.Context, containerName string) (string, error) {
	containerName = "demimine-" + sanitizeName(containerName)
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
	containerName = "demimine-" + sanitizeName(containerName)

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

	var lines []string
	buf := make([]byte, 8192)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]
			if len(data) > 8 {
				data = data[8:]
			}
			for _, line := range strings.Split(string(data), "\n") {
				if line != "" {
					lines = append(lines, strings.TrimSuffix(line, "\r"))
				}
			}
		}
		if err != nil {
			break
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
	name = "demimine-" + sanitizeName(name)
	_, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Client) GetContainerID(ctx context.Context, name string) (string, error) {
	name = "demimine-" + sanitizeName(name)
	ctr, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", err
	}
	return ctr.ID, nil
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	return strings.Trim(result.String(), "-")
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

func (c *Client) SyncServerStatus(ctx context.Context, db *sql.DB) {
	rows, err := db.Query("SELECT id, name FROM servers WHERE status = 'running'")
	if err != nil {
		log.Printf("Failed to query running servers for status sync: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Printf("Failed to scan server row: %v", err)
			continue
		}

		exists, _ := c.ContainerExists(ctx, name)
		if !exists {
			_, err := db.Exec("UPDATE servers SET status = 'stopped' WHERE id = ?", id)
			if err != nil {
				log.Printf("Failed to update server %d status: %v", id, err)
			} else {
				log.Printf("Synced server %d (%s): running -> stopped (container gone)", id, name)
			}
		}
	}
}
