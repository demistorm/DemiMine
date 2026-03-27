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
	if s.manager.AlreadyCheckedToday() {
		return
	}

	settings := s.manager.GetSettings()
	backupTimeStr, ok := settings["backup_time"]
	if !ok || backupTimeStr == "" {
		backupTimeStr = "03:00"
	}

	backupTime, err := time.Parse("15:04", backupTimeStr)
	if err != nil {
		log.Printf("Backup scheduler: invalid backup_time format: %v", err)
		return
	}

	now := time.Now()
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), backupTime.Hour(), backupTime.Minute(), 0, 0, now.Location())
	windowEnd := windowStart.Add(5 * time.Minute)

	if !now.After(windowStart) || !now.Before(windowEnd) {
		return
	}

	s.manager.MarkCheckedToday()

	if !s.manager.ShouldRunBackup() {
		return
	}

	log.Println("Starting scheduled backup...")
	_, err = s.manager.CreateSnapshot(context.Background(), nil)
	if err != nil {
		log.Printf("Scheduled backup failed: %v\n", err)
	} else {
		log.Println("Scheduled backup completed successfully")
	}
}
