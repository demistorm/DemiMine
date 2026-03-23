package rcon

import (
	"log"
	"sync"
	"time"
)

type Pool struct {
	connections      map[string]*Client
	connectionsMutex sync.RWMutex
	password         string
}

func NewPool(password string) *Pool {
	return &Pool{
		connections: make(map[string]*Client),
		password:    password,
	}
}

func (p *Pool) GetConnection(containerName string) (*Client, error) {
	p.connectionsMutex.Lock()
	defer p.connectionsMutex.Unlock()

	if client, ok := p.connections[containerName]; ok {
		if client.IsConnected() {
			return client, nil
		}
		delete(p.connections, containerName)
	}

	client := NewClient(containerName+":39521", p.password)
	if err := client.Connect(); err != nil {
		return nil, err
	}

	p.connections[containerName] = client
	return client, nil
}

func (p *Pool) Release(containerName string) {
	p.connectionsMutex.Lock()
	defer p.connectionsMutex.Unlock()

	if client, ok := p.connections[containerName]; ok {
		delete(p.connections, containerName)
		client.Close()
	}
}

func (p *Pool) Close(containerName string) {
	p.Release(containerName)
}

func (p *Pool) CloseAll() {
	p.connectionsMutex.Lock()
	defer p.connectionsMutex.Unlock()

	for name, client := range p.connections {
		client.Close()
		delete(p.connections, name)
	}
}

func (p *Pool) TestConnection(containerName string) error {
	client, err := p.GetConnection(containerName)
	if err != nil {
		return err
	}

	_, err = client.SendCommand("/")
	if err != nil {
		p.Close(containerName)
		return err
	}

	return nil
}

func (p *Pool) MaintainConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.connectionsMutex.RLock()
		containers := make([]string, 0, len(p.connections))
		for name := range p.connections {
			containers = append(containers, name)
		}
		p.connectionsMutex.RUnlock()

		for _, name := range containers {
			if err := p.TestConnection(name); err != nil {
				log.Printf("[RCON] Connection test failed for %s: %v", name, err)
			}
		}
	}
}
