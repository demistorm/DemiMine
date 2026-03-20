package backup

import (
	"context"
	"log"
	"time"
)

type Scheduler struct {
	manager *Manager
	ticker  *time.Ticker
	stopCh  chan struct{}
}

func NewScheduler(manager *Manager) *Scheduler {
	return &Scheduler{
		manager: manager,
		stopCh:  make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.ticker = time.NewTicker(1 * time.Hour)
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
	settings := s.manager.GetSettings()
	backupTime, ok := settings["backup_time"]
	if !ok || backupTime == "" {
		backupTime = "03:00"
	}

	currentTime := time.Now().Format("15:04")
	if currentTime != backupTime {
		return
	}

	if !s.manager.ShouldRunBackup() {
		return
	}

	log.Println("Starting scheduled backup...")
	_, err := s.manager.CreateSnapshot(context.Background(), nil)
	if err != nil {
		log.Printf("Scheduled backup failed: %v\n", err)
	} else {
		log.Println("Scheduled backup completed successfully")
	}
}
