package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Client struct {
	cli         *client.Client
	networkName string
}

func NewClient(networkName string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	
	return &Client{
		cli:         cli,
		networkName: networkName,
	}, nil
}

type ServerContainerConfig struct {
	Name        string
	ServerType  string
	Version     string
	RAMMB       int
	JavaPath    string
	ServerPath  string
	HostPort    int
}

func (c *Client) CreateServerContainer(ctx context.Context, cfg ServerContainerConfig) error {
	imageName := "eclipse-temurin:21-jre-alpine"
	
	cmd := []string{
		fmt.Sprintf("-Xmx%dM", cfg.RAMMB),
		fmt.Sprintf("-Xms%dM", cfg.RAMMB/2),
		"-jar", "server.jar",
		"nogui",
	}
	
	containerConfig := &container.Config{
		Image:      imageName,
		Cmd:        cmd,
		WorkingDir: "/server",
		ExposedPorts: nat.PortSet{
			"25565/tcp": {},
		},
		Labels: map[string]string{
			"demimine.managed":   "true",
			"demimine.server_id": cfg.Name,
			"demimine.type":      "server",
		},
	}
	
	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:/server:rw", cfg.ServerPath),
		},
		Resources: container.Resources{
			Memory: int64(cfg.RAMMB) * 1024 * 1024,
		},
		AutoRemove:   false,
		NetworkMode:  container.NetworkMode(c.networkName),
	}
	
	if cfg.HostPort > 0 {
		hostConfig.PortBindings = nat.PortMap{
			"25565/tcp": []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.HostPort)},
			},
		}
	}
	
	networkConfig := &network.NetworkingConfig{}
	
	_, err := c.cli.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, "demimine-"+cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	
	return nil
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStart(ctx, "demimine-"+name, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerStop(ctx, "demimine-"+name, container.StopOptions{Timeout: &timeout})
}

func (c *Client) RemoveContainer(ctx context.Context, name string) error {
	return c.cli.ContainerRemove(ctx, "demimine-"+name, container.RemoveOptions{Force: true})
}

func (c *Client) ContainerIsRunning(ctx context.Context, name string) (bool, error) {
	stats, err := c.cli.ContainerInspect(ctx, "demimine-"+name)
	if err != nil {
		return false, err
	}
	return stats.State != nil && stats.State.Running, nil
}

func (c *Client) Close() error {
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}
