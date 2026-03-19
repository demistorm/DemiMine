package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

type ProxyContainerConfig struct {
	Name        string
	HostPort    int
	ProxyPath   string
	NetworkName string
	RAMMB       int
}

func (c *Client) CreateProxyContainer(ctx context.Context, cfg ProxyContainerConfig) (string, error) {
	if c == nil {
		return "", fmt.Errorf("client is nil")
	}
	if c.cli == nil {
		return "", fmt.Errorf("docker client is nil")
	}

	containerName := "demimine-proxy-" + sanitizeName(cfg.Name)

	existing, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
	} else if existing.ID != "" {
		_ = c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
			Force: true,
		})
	}

	javaImage := "eclipse-temurin:21-jre-alpine"

	_, _, err = c.cli.ImageInspectWithRaw(ctx, javaImage)
	if err != nil {
		reader, err := c.cli.ImagePull(ctx, javaImage, image.PullOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to pull image %s: %w", javaImage, err)
		}
		io.Copy(io.Discard, reader)
		reader.Close()
	}

	ramMB := cfg.RAMMB
	if ramMB == 0 {
		ramMB = 512
	}

	env := []string{
		"TERM=xterm",
		"TZ=America/Chicago",
	}

	cmd := []string{"java", fmt.Sprintf("-Xmx%dM", ramMB), "-jar", "velocity.jar"}

	proxyPort := fmt.Sprintf("%d", cfg.HostPort)
	config := &container.Config{
		Image:        javaImage,
		Hostname:     containerName,
		Cmd:          cmd,
		Env:          env,
		WorkingDir:   "/proxy",
		ExposedPorts: nat.PortSet{nat.Port(proxyPort + "/tcp"): {}},
		Labels: map[string]string{
			"demimine.managed":  "true",
			"demimine.proxy_id": cfg.Name,
			"demimine.type":     "proxy",
		},
		Tty:       true,
		OpenStdin: true,
	}

	hostConfig := &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:/proxy:rw", cfg.ProxyPath)},
		Resources: container.Resources{
			Memory: int64(ramMB) * 1024 * 1024,
		},
		AutoRemove: true,
		RestartPolicy: container.RestartPolicy{
			Name: "no",
		},
		PortBindings: nat.PortMap{
			nat.Port(proxyPort + "/tcp"): []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: proxyPort},
			},
		},
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			cfg.NetworkName: {},
		},
	}

	created, err := c.cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create proxy container: %w", err)
	}

	return created.ID, nil
}

func (c *Client) StartProxyContainer(ctx context.Context, name string, cfg *ProxyContainerConfig) error {
	containerName := "demimine-proxy-" + sanitizeName(name)

	exists, err := c.ContainerExists(ctx, "proxy-"+name)
	if err != nil {
		return fmt.Errorf("failed to check if proxy container exists: %w", err)
	}

	if !exists {
		if cfg == nil {
			return fmt.Errorf("proxy container %s does not exist and no config provided", containerName)
		}
		_, err = c.CreateProxyContainer(ctx, *cfg)
		if err != nil {
			return fmt.Errorf("failed to create proxy container: %w", err)
		}
	}

	return c.cli.ContainerStart(ctx, containerName, container.StartOptions{})
}

func (c *Client) StopProxyContainer(ctx context.Context, name string, timeout *int) error {
	if timeout == nil {
		t := 30
		timeout = &t
	}
	containerName := "demimine-proxy-" + sanitizeName(name)
	return c.cli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: timeout})
}

func (c *Client) GetProxyContainerStatus(ctx context.Context, name string) (string, error) {
	containerName := "demimine-proxy-" + sanitizeName(name)
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

func (c *Client) GetProxyContainerLogs(ctx context.Context, name string, tail int) ([]string, error) {
	containerName := "demimine-proxy-" + sanitizeName(name)

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

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read proxy logs: %w", err)
	}

	var lines []string
	for _, line := range splitLines(string(data)) {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines, nil
}

func (c *Client) ProxyContainerExists(ctx context.Context, name string) (bool, error) {
	name = "demimine-proxy-" + sanitizeName(name)
	_, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Client) GetProxyContainerID(ctx context.Context, name string) (string, error) {
	name = "demimine-proxy-" + sanitizeName(name)
	ctr, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", err
	}
	return ctr.ID, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
