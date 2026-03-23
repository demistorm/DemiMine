package spark

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/demimine/manager/internal/rcon"
)

type Service struct {
	rconPool  *rcon.Pool
	hub       Hub
	urlWaiter ProfilerURLWaiter
}

type Hub interface {
	Broadcast(data []byte)
}

type ProfilerURLWaiter interface {
	WaitForProfilerURL(serverID int64) <-chan string
	CancelProfilerURLWait(serverID int64)
}

func NewService(rconPool *rcon.Pool, hub Hub, urlWaiter ProfilerURLWaiter) *Service {
	return &Service{
		rconPool:  rconPool,
		hub:       hub,
		urlWaiter: urlWaiter,
	}
}

func (s *Service) StartProfiler(containerName string) error {
	conn, err := s.rconPool.GetConnection(containerName)
	if err != nil {
		return err
	}

	resp, err := conn.SendCommand("spark profiler start")
	if err != nil {
		return err
	}

	if strings.Contains(resp, "Already running") {
		return nil
	}

	return nil
}

func (s *Service) StopProfiler(containerName, serverID string) (string, error) {
	if s.urlWaiter == nil {
		conn, err := s.rconPool.GetConnection(containerName)
		if err != nil {
			return "", err
		}

		resp, err := conn.SendCommand("spark profiler stop")
		if err != nil {
			return "", err
		}

		url := extractProfileURL(resp)
		if url == "" {
			return "", nil
		}

		return url, nil
	}

	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid server ID: %w", err)
	}

	urlChan := s.urlWaiter.WaitForProfilerURL(serverIDInt)
	defer s.urlWaiter.CancelProfilerURLWait(serverIDInt)

	conn, err := s.rconPool.GetConnection(containerName)
	if err != nil {
		return "", err
	}

	_, _ = conn.SendCommand("spark profiler stop")

	select {
	case url := <-urlChan:
		return url, nil
	case <-time.After(10 * time.Second):
		log.Printf("[Spark] Timeout waiting for profiler URL from server %s", serverID)
		return "", nil
	}
}

func extractProfileURL(resp string) string {
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		if strings.Contains(line, "spark.lucko.me") {
			start := strings.Index(line, "https://spark.lucko.me/")
			if start != -1 {
				end := start + 24
				for end < len(line) && (line[end] >= 'a' && line[end] <= 'z' || line[end] >= 'A' && line[end] <= 'Z' || line[end] >= '0' && line[end] <= '9' || line[end] == '-') {
					end++
				}
				return line[start:end]
			}
		}
	}
	return ""
}
