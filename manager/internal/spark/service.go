package spark

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/demimine/manager/internal/rcon"
)

type Stats struct {
	TPS          float64
	MemoryUsedMB int64
	MemoryMaxMB  int64
	CPUPercent   float64
}

type Service struct {
	rconPool      *rcon.Pool
	hub           Hub
	urlWaiter     ProfilerURLWaiter
	active        map[string]context.CancelFunc
	mu            sync.RWMutex
	sparkDisabled map[string]bool
}

type Hub interface {
	Broadcast(data []byte)
}

type ProfilerURLWaiter interface {
	WaitForProfilerURL(containerName string) <-chan string
	CancelProfilerURLWait(containerName string)
}

func NewService(rconPool *rcon.Pool, hub Hub, urlWaiter ProfilerURLWaiter) *Service {
	return &Service{
		rconPool:      rconPool,
		hub:           hub,
		urlWaiter:     urlWaiter,
		active:        make(map[string]context.CancelFunc),
		sparkDisabled: make(map[string]bool),
	}
}

func (s *Service) StartMonitoring(containerName string, targetID int64, targetType string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.active[containerName]; ok {
		return
	}

	var conn *rcon.Client
	var err error

	for attempt := 1; attempt <= 5; attempt++ {
		conn, err = s.rconPool.GetConnection(containerName)
		if err == nil {
			break
		}
		log.Printf("[Spark] RCON connection attempt %d/5 failed for %s: %v", attempt, containerName, err)
		if attempt < 5 {
			time.Sleep(5 * time.Second)
		}
	}

	if err != nil {
		log.Printf("[Spark] Cannot connect to %s RCON after 5 attempts: %v (stats disabled)", containerName, err)
		s.broadcastError(targetID, targetType, "RCON connection failed")
		return
	}

	var sparkResp string
	var sparkErr error
	for attempt := 1; attempt <= 10; attempt++ {
		sparkResp, sparkErr = conn.SendCommand("spark tps")
		if sparkErr != nil {
			log.Printf("[Spark] Spark check attempt %d/10 failed for %s: %v", attempt, containerName, sparkErr)
			if attempt < 10 {
				time.Sleep(3 * time.Second)
			}
			continue
		}

		if strings.Contains(sparkResp, "Unknown command") || strings.Contains(sparkResp, "Unknown or incomplete command") {
			if !s.sparkDisabled[containerName] {
				log.Printf("[Spark] Spark not installed on %s, skipping monitoring", containerName)
				s.sparkDisabled[containerName] = true
				s.broadcastError(targetID, targetType, "Spark not installed")
			}
			s.rconPool.Close(containerName)
			return
		}

		if strings.ContainsAny(sparkResp, "0123456789") && strings.TrimSpace(sparkResp) != "" {
			break
		}

		if attempt < 10 {
			log.Printf("[Spark] Spark not ready yet on %s (attempt %d/10), retrying...", containerName, attempt)
			time.Sleep(3 * time.Second)
		}
	}

	if sparkErr != nil {
		log.Printf("[Spark] Failed to check Spark on %s after 10 attempts: %v", containerName, sparkErr)
		s.broadcastError(targetID, targetType, "Spark unavailable")
		s.rconPool.Close(containerName)
		return
	}

	if strings.Contains(sparkResp, "Unknown command") || strings.Contains(sparkResp, "Unknown or incomplete command") {
		if !s.sparkDisabled[containerName] {
			log.Printf("[Spark] Spark not installed on %s, skipping monitoring", containerName)
			s.sparkDisabled[containerName] = true
			s.broadcastError(targetID, targetType, "Spark not installed")
		}
		s.rconPool.Close(containerName)
		return
	}

	if !strings.ContainsAny(sparkResp, "0123456789") || strings.TrimSpace(sparkResp) == "" {
		log.Printf("[Spark] Spark not responding on %s after 10 attempts, skipping monitoring", containerName)
		s.broadcastError(targetID, targetType, "Spark unavailable")
		s.rconPool.Close(containerName)
		return
	}

	delete(s.sparkDisabled, containerName)
	s.sparkDisabled[containerName] = false
	log.Printf("[Spark] Started monitoring %s (type: %s, id: %d)", containerName, targetType, targetID)

	ctx, cancel := context.WithCancel(context.Background())
	s.active[containerName] = cancel

	go s.monitorLoop(ctx, containerName, targetID, targetType)
}

func (s *Service) StopMonitoring(containerName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, ok := s.active[containerName]; ok {
		cancel()
		delete(s.active, containerName)
	}
}

func (s *Service) monitorLoop(ctx context.Context, containerName string, targetID int64, targetType string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Spark] Stopped monitoring %s", containerName)
			return
		case <-ticker.C:
			stats, err := s.collectStats(containerName)
			if err != nil {
				log.Printf("[Spark] Failed to collect stats from %s: %v", containerName, err)
				continue
			}
			s.broadcastStats(targetID, targetType, stats)
		}
	}
}

func (s *Service) collectStats(containerName string) (*Stats, error) {
	conn, err := s.rconPool.GetConnection(containerName)
	if err != nil {
		return nil, err
	}

	tpsResp, err := conn.SendCommand("spark tps")
	if err != nil {
		return nil, err
	}

	healthResp, err := conn.SendCommand("spark health")
	if err != nil {
		return nil, err
	}

	stats := &Stats{
		TPS:          parseTPS(tpsResp),
		MemoryUsedMB: parseMemoryUsed(healthResp),
		MemoryMaxMB:  parseMemoryMax(healthResp),
		CPUPercent:   parseCPU(healthResp),
	}

	return stats, nil
}

func (s *Service) broadcastStats(targetID int64, targetType string, stats *Stats) {
	data := map[string]interface{}{
		"type":        "resources",
		"server_id":   targetID,
		"tps":         stats.TPS,
		"memory_mb":   stats.MemoryUsedMB,
		"cpu_percent": stats.CPUPercent,
	}

	if targetType == "proxy" {
		data["proxy_id"] = targetID
		delete(data, "server_id")
	}

	encoded, err := encodeMessage(data)
	if err != nil {
		log.Printf("[Spark] Failed to encode stats: %v", err)
		return
	}

	s.hub.Broadcast(encoded)
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

func (s *Service) StopProfiler(containerName string) (string, error) {
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

	urlChan := s.urlWaiter.WaitForProfilerURL(containerName)
	defer s.urlWaiter.CancelProfilerURLWait(containerName)

	conn, err := s.rconPool.GetConnection(containerName)
	if err != nil {
		return "", err
	}

	_, _ = conn.SendCommand("spark profiler stop")

	select {
	case url := <-urlChan:
		return url, nil
	case <-time.After(10 * time.Second):
		log.Printf("[Spark] Timeout waiting for profiler URL from %s", containerName)
		return "", nil
	}
}

func (s *Service) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cancel := range s.active {
		cancel()
	}
	s.active = make(map[string]context.CancelFunc)
}

func (s *Service) broadcastError(targetID int64, targetType string, errMsg string) {
	data := map[string]interface{}{
		"type":  "spark_error",
		"error": errMsg,
	}
	if targetType == "proxy" {
		data["proxy_id"] = targetID
	} else {
		data["server_id"] = targetID
	}

	encoded, _ := encodeMessage(data)
	s.hub.Broadcast(encoded)
}

func parseTPS(resp string) float64 {
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		if strings.Contains(line, "TPS:") || strings.Contains(line, "Tick Rate:") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if (part == "TPS:" || part == "Rate:") && i+1 < len(parts) {
					val, err := strconv.ParseFloat(strings.TrimSuffix(parts[i+1], ","), 64)
					if err == nil {
						return val
					}
				}
			}
		}
	}
	return 20.0
}

func parseMemoryUsed(resp string) int64 {
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "memory") {
			if strings.Contains(line, "used") || strings.Contains(line, "/") {
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasSuffix(part, "MB") || strings.HasSuffix(part, "MiB") {
						val, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSuffix(part, "MiB"), "MB"), 10, 64)
						if err == nil && val > 0 {
							return val
						}
					}
				}
			}
		}
	}
	return 0
}

func parseMemoryMax(resp string) int64 {
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "memory") {
			if strings.Contains(line, "max") || strings.Contains(line, "/") {
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasSuffix(part, "MB") || strings.HasSuffix(part, "MiB") {
						val, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSuffix(part, "MiB"), "MB"), 10, 64)
						if err == nil && val > 0 {
							return val
						}
					}
				}
			}
		}
	}
	return 0
}

func parseCPU(resp string) float64 {
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "cpu") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.HasSuffix(part, "%") {
					val, err := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
					if err == nil {
						return val
					}
				}
				if i+1 < len(parts) && (strings.ToLower(parts[i]) == "cpu:" || strings.ToLower(parts[i]) == "cpu") {
					val, err := strconv.ParseFloat(strings.TrimSuffix(parts[i+1], "%"), 64)
					if err == nil {
						return val
					}
				}
			}
		}
	}
	return 0.0
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

func encodeMessage(data map[string]interface{}) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{"type":"`)
	b.WriteString(data["type"].(string))
	delete(data, "type")

	if serverID, ok := data["server_id"]; ok {
		b.WriteString(`","server_id":`)
		b.WriteString(strconv.FormatInt(serverID.(int64), 10))
		delete(data, "server_id")
	}

	if proxyID, ok := data["proxy_id"]; ok {
		b.WriteString(`","proxy_id":`)
		b.WriteString(strconv.FormatInt(proxyID.(int64), 10))
		delete(data, "proxy_id")
	}

	if tps, ok := data["tps"]; ok {
		b.WriteString(`","tps":`)
		b.WriteString(strconv.FormatFloat(tps.(float64), 'f', 1, 64))
		delete(data, "tps")
	}

	if memoryMB, ok := data["memory_mb"]; ok {
		b.WriteString(`","memory_mb":`)
		b.WriteString(strconv.FormatInt(memoryMB.(int64), 10))
		delete(data, "memory_mb")
	}

	if cpuPercent, ok := data["cpu_percent"]; ok {
		b.WriteString(`","cpu_percent":`)
		b.WriteString(strconv.FormatFloat(cpuPercent.(float64), 'f', 1, 64))
		delete(data, "cpu_percent")
	}

	b.WriteString(`}`)
	return []byte(b.String()), nil
}
