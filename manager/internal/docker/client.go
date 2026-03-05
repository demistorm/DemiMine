package docker

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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

	ctx := context.Background()
	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to ping docker daemon: %w", err)
	}

	log.Println("Docker client connected successfully")

	return &Client{
		cli:         cli,
		networkName: networkName,
	}, nil
}

func (c *Client) Close() error {
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}

func (c *Client) SendCommand(ctx context.Context, containerName, command string) error {
	execConfig := container.ExecOptions{
		Cmd:          []string{"screen", "-S", "minecraft", "-X", "stuff", command + "\n"},
		AttachStdout: false,
		AttachStderr: false,
	}

	execResp, err := c.cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return err
	}

	return c.cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{})
}
