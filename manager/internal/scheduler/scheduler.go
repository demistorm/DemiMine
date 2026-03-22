package scheduler

import (
	"database/sql"
	"log"
	"time"

	"github.com/demimine/manager/internal/api/handlers"
)

type Scheduler struct {
	db            *sql.DB
	serverHandler *handlers.ServerHandler
	proxyHandler  *handlers.ProxyHandler
	ticker        *time.Ticker
	stopCh        chan struct{}
}

func NewScheduler(db *sql.DB) *Scheduler {
	return &Scheduler{
		db:     db,
		stopCh: make(chan struct{}),
	}
}

func (s *Scheduler) SetHandlers(sh *handlers.ServerHandler, ph *handlers.ProxyHandler) {
	s.serverHandler = sh
	s.proxyHandler = ph
}

func (s *Scheduler) Start() {
	s.ticker = time.NewTicker(5 * time.Minute)
	go s.run()
}

func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
}

func (s *Scheduler) run() {
	for {
		select {
		case <-s.ticker.C:
			s.checkAndRun()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) checkAndRun() {
	s.checkServers()
	s.checkProxies()
}

func (s *Scheduler) checkServers() {
	currentTime := time.Now()

	rows, err := s.db.Query(`
		SELECT id, name, status, scheduled_start, scheduled_stop 
		FROM servers 
		WHERE scheduled_start IS NOT NULL OR scheduled_stop IS NOT NULL`)
	if err != nil {
		log.Printf("Scheduler: error querying servers: %v", err)
		return
	}
	defer rows.Close()

	type serverAction struct {
		id             int64
		name           string
		status         string
		scheduledStart sql.NullString
		scheduledStop  sql.NullString
	}

	var actions []serverAction
	for rows.Next() {
		var sa serverAction
		if err := rows.Scan(&sa.id, &sa.name, &sa.status, &sa.scheduledStart, &sa.scheduledStop); err != nil {
			log.Printf("Scheduler: error scanning server %s: %v", sa.name, err)
			continue
		}
		actions = append(actions, sa)
	}

	log.Printf("Scheduler: checking %d servers for scheduled actions", len(actions))

	for _, sa := range actions {
		if sa.scheduledStart.Valid && sa.status == "stopped" {
			scheduledTime, err := time.Parse("15:04", sa.scheduledStart.String)
			if err == nil {
				windowStart := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), scheduledTime.Hour(), scheduledTime.Minute(), 0, 0, currentTime.Location())
				windowEnd := windowStart.Add(5 * time.Minute)

				if currentTime.After(windowStart) && currentTime.Before(windowEnd) {
					log.Printf("Scheduler: triggering scheduled start for server %s (id=%d)", sa.name, sa.id)
					go func(id int64, name string) {
						if err := s.serverHandler.StartByID(id); err != nil {
							log.Printf("Scheduler: failed to start server %s (id=%d): %v", name, id, err)
						} else {
							log.Printf("Scheduler: successfully started server %s (id=%d)", name, id)
						}
					}(sa.id, sa.name)
				}
			} else {
				log.Printf("Scheduler: invalid scheduled_start time format for server %s: %v", sa.name, err)
			}
		}

		if sa.scheduledStop.Valid && sa.status == "running" {
			scheduledTime, err := time.Parse("15:04", sa.scheduledStop.String)
			if err == nil {
				windowStart := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), scheduledTime.Hour(), scheduledTime.Minute(), 0, 0, currentTime.Location())
				windowEnd := windowStart.Add(5 * time.Minute)

				if currentTime.After(windowStart) && currentTime.Before(windowEnd) {
					log.Printf("Scheduler: triggering scheduled stop for server %s (id=%d)", sa.name, sa.id)
					go func(id int64, name string) {
						if err := s.serverHandler.StopByID(id); err != nil {
							log.Printf("Scheduler: failed to stop server %s (id=%d): %v", name, id, err)
						} else {
							log.Printf("Scheduler: successfully stopped server %s (id=%d)", name, id)
						}
					}(sa.id, sa.name)
				}
			} else {
				log.Printf("Scheduler: invalid scheduled_stop time format for server %s: %v", sa.name, err)
			}
		}
	}
}

func (s *Scheduler) checkProxies() {
	currentTime := time.Now()

	rows, err := s.db.Query(`
		SELECT id, name, status, scheduled_start, scheduled_stop 
		FROM proxies 
		WHERE scheduled_start IS NOT NULL OR scheduled_stop IS NOT NULL`)
	if err != nil {
		log.Printf("Scheduler: error querying proxies: %v", err)
		return
	}
	defer rows.Close()

	type proxyAction struct {
		id             int64
		name           string
		status         string
		scheduledStart sql.NullString
		scheduledStop  sql.NullString
	}

	var actions []proxyAction
	for rows.Next() {
		var pa proxyAction
		if err := rows.Scan(&pa.id, &pa.name, &pa.status, &pa.scheduledStart, &pa.scheduledStop); err != nil {
			log.Printf("Scheduler: error scanning proxy %s: %v", pa.name, err)
			continue
		}
		actions = append(actions, pa)
	}

	log.Printf("Scheduler: checking %d proxies for scheduled actions", len(actions))

	for _, pa := range actions {
		if pa.scheduledStart.Valid && pa.status == "stopped" {
			scheduledTime, err := time.Parse("15:04", pa.scheduledStart.String)
			if err == nil {
				windowStart := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), scheduledTime.Hour(), scheduledTime.Minute(), 0, 0, currentTime.Location())
				windowEnd := windowStart.Add(5 * time.Minute)

				if currentTime.After(windowStart) && currentTime.Before(windowEnd) {
					log.Printf("Scheduler: triggering scheduled start for proxy %s (id=%d)", pa.name, pa.id)
					go func(id int64, name string) {
						if err := s.proxyHandler.StartByID(id); err != nil {
							log.Printf("Scheduler: failed to start proxy %s (id=%d): %v", name, id, err)
						} else {
							log.Printf("Scheduler: successfully started proxy %s (id=%d)", name, id)
						}
					}(pa.id, pa.name)
				}
			} else {
				log.Printf("Scheduler: invalid scheduled_start time format for proxy %s: %v", pa.name, err)
			}
		}

		if pa.scheduledStop.Valid && pa.status == "running" {
			scheduledTime, err := time.Parse("15:04", pa.scheduledStop.String)
			if err == nil {
				windowStart := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), scheduledTime.Hour(), scheduledTime.Minute(), 0, 0, currentTime.Location())
				windowEnd := windowStart.Add(5 * time.Minute)

				if currentTime.After(windowStart) && currentTime.Before(windowEnd) {
					log.Printf("Scheduler: triggering scheduled stop for proxy %s (id=%d)", pa.name, pa.id)
					go func(id int64, name string) {
						if err := s.proxyHandler.StopByID(id); err != nil {
							log.Printf("Scheduler: failed to stop proxy %s (id=%d): %v", name, id, err)
						} else {
							log.Printf("Scheduler: successfully stopped proxy %s (id=%d)", name, id)
						}
					}(pa.id, pa.name)
				}
			} else {
				log.Printf("Scheduler: invalid scheduled_stop time format for proxy %s: %v", pa.name, err)
			}
		}
	}
}
