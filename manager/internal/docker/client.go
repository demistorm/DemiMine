package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
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

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

func (c *Client) StreamLogs(ctx context.Context, containerName string, logChan chan<- string) error {
	containerName = strings.TrimPrefix(containerName, "/")
	if !strings.HasPrefix(containerName, "demimine-") {
		containerName = "demimine-" + SanitizeName(containerName)
	}

	cfg := container.AttachOptions{
		Stdin:  false,
		Stdout: true,
		Stderr: true,
		Stream: true,
	}

	hijacked, err := c.cli.ContainerAttach(ctx, containerName, cfg)
	if err != nil {
		return fmt.Errorf("failed to attach to container: %w", err)
	}
	defer hijacked.Close()

	go func() {
		<-ctx.Done()
		hijacked.Close()
	}()

	scanner := bufio.NewScanner(hijacked.Reader)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			select {
			case logChan <- line:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return scanner.Err()
}

func (c *Client) ContainerEvents(ctx context.Context, eventChan chan<- events.Message) error {
	filter := filters.NewArgs()
	filter.Add("type", "container")

	dockerEvents, errs := c.cli.Events(ctx, events.ListOptions{
		Filters: filter,
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errs:
				if err != nil && err != io.EOF {
					log.Printf("Container events error: %v", err)
				}
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-dockerEvents:
			eventChan <- event
		}
	}
}
