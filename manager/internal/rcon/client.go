package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	packetTypeAuth     = 3
	packetTypeCommand  = 2
	packetTypeResponse = 0
	defaultTimeout     = 10 * time.Second
)

type Packet struct {
	ID      int32
	Type    int32
	Payload []byte
}

type Client struct {
	conn       net.Conn
	addr       string
	password   string
	timeout    time.Duration
	mu         sync.Mutex
	authed     bool
	packetID   int32
	closed     bool
	reconnects int
}

func NewClient(addr, password string) *Client {
	return &Client{
		addr:     addr,
		password: password,
		timeout:  defaultTimeout,
		packetID: 1,
	}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("failed to connect to RCON: %w", err)
	}

	c.conn = conn

	if err := c.authenticate(); err != nil {
		c.conn.Close()
		c.conn = nil
		return fmt.Errorf("RCON authentication failed: %w", err)
	}

	c.authed = true
	return nil
}

func (c *Client) authenticate() error {
	authPacket := Packet{
		ID:      c.nextPacketID(),
		Type:    packetTypeAuth,
		Payload: []byte(c.password),
	}

	if err := c.sendPacket(&authPacket); err != nil {
		return err
	}

	resp, err := c.readPacket()
	if err != nil {
		return err
	}

	if resp.ID != authPacket.ID {
		return errors.New("invalid packet ID in auth response")
	}

	if resp.Type == packetTypeResponse && len(resp.Payload) == 0 {
		return errors.New("empty auth response")
	}

	return nil
}

func (c *Client) SendCommand(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.closed {
		if err := c.reconnect(); err != nil {
			return "", err
		}
	}

	cmdPacket := Packet{
		ID:      c.nextPacketID(),
		Type:    packetTypeCommand,
		Payload: []byte(cmd),
	}

	if err := c.sendPacket(&cmdPacket); err != nil {
		return "", err
	}

	var fullResponse bytes.Buffer
	for {
		resp, err := c.readPacket()
		if err != nil {
			return "", err
		}

		fullResponse.Write(resp.Payload)

		if resp.ID == cmdPacket.ID {
			break
		}
	}

	return string(fullResponse.Bytes()), nil
}

func (c *Client) reconnect() error {
	if c.conn != nil {
		c.conn.Close()
	}

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("failed to reconnect to RCON: %w", err)
	}

	c.conn = conn
	c.closed = false
	c.reconnects++

	if err := c.authenticate(); err != nil {
		c.conn.Close()
		c.conn = nil
		return fmt.Errorf("RCON re-authentication failed: %w", err)
	}

	c.authed = true
	return nil
}

func (c *Client) sendPacket(p *Packet) error {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}

	payloadLen := len(p.Payload)
	packetLen := 4 + 4 + payloadLen + 2

	buf := make([]byte, 4+packetLen)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(packetLen))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(p.ID))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(p.Type))
	copy(buf[12:12+payloadLen], p.Payload)
	buf[12+payloadLen] = 0
	buf[12+payloadLen+1] = 0

	if _, err := c.conn.Write(buf); err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}

	return nil
}

func (c *Client) readPacket() (*Packet, error) {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, fmt.Errorf("failed to read packet length: %w", err)
	}

	packetLen := binary.LittleEndian.Uint32(lenBuf)
	if packetLen < 10 || packetLen > 4096 {
		return nil, fmt.Errorf("invalid packet length: %d", packetLen)
	}

	payloadLen := packetLen - 10
	buf := make([]byte, 4+4+payloadLen+2)

	if _, err := io.ReadFull(c.conn, buf); err != nil {
		return nil, fmt.Errorf("failed to read packet payload: %w", err)
	}

	packet := &Packet{
		ID:      int32(binary.LittleEndian.Uint32(buf[0:4])),
		Type:    int32(binary.LittleEndian.Uint32(buf[4:8])),
		Payload: buf[8 : 8+payloadLen],
	}

	return packet, nil
}

func (c *Client) nextPacketID() int32 {
	c.packetID++
	return c.packetID
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && !c.closed
}

func (c *Client) GetReconnectCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reconnects
}
