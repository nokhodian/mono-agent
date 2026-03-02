package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
)

// ActionExecutor interface for running actions
type ActionExecutor interface {
	RunSingle(ctx context.Context, action *ScheduledAction) error
}

// ActionStore interface for getting actions
type ActionStore interface {
	GetAction(id string) (*ScheduledAction, error)
	UpdateActionState(id, state string) error
}

// ScheduledAction contains the fields needed for scheduling
type ScheduledAction struct {
	ID                string
	Type              string
	State             string
	TargetPlatform    string
	ScheduledDate     string
	ExecutionInterval int
	StartDate         string
	EndDate           string
}

// Scheduler manages recurring and scheduled action execution.
type Scheduler struct {
	cron     *cron.Cron
	executor ActionExecutor
	store    ActionStore
	logger   zerolog.Logger
	jobs     map[string]cron.EntryID
}

func NewScheduler(executor ActionExecutor, store ActionStore, logger zerolog.Logger) *Scheduler {
	return &Scheduler{
		cron:     cron.New(cron.WithSeconds()),
		executor: executor,
		store:    store,
		logger:   logger,
		jobs:     make(map[string]cron.EntryID),
	}
}

// ScheduleAction adds a recurring action with a cron expression.
func (s *Scheduler) ScheduleAction(ctx context.Context, actionID, cronExpr string) error {
	entryID, err := s.cron.AddFunc(cronExpr, func() {
		action, err := s.store.GetAction(actionID)
		if err != nil {
			s.logger.Error().Err(err).Str("actionID", actionID).Msg("failed to load scheduled action")
			return
		}
		if err := s.executor.RunSingle(ctx, action); err != nil {
			s.logger.Error().Err(err).Str("actionID", actionID).Msg("scheduled action execution failed")
		}
	})
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	s.jobs[actionID] = entryID
	s.logger.Info().Str("actionID", actionID).Str("cron", cronExpr).Msg("action scheduled")
	return nil
}

// RemoveSchedule removes a scheduled action.
func (s *Scheduler) RemoveSchedule(actionID string) {
	if entryID, ok := s.jobs[actionID]; ok {
		s.cron.Remove(entryID)
		delete(s.jobs, actionID)
	}
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() context.Context {
	return s.cron.Stop()
}

// NextPeriod calculates the next execution time for a recurring action within a time window.
func NextPeriod(start, end time.Time, pollIntervalMinutes int) *time.Time {
	now := time.Now().Truncate(time.Minute)

	if now.Before(start) {
		return &start
	}
	if now.After(end) {
		return nil
	}

	minutesSinceStart := int(now.Sub(start).Minutes())
	nextInterval := ((minutesSinceStart / pollIntervalMinutes) + 1) * pollIntervalMinutes
	nextTime := start.Add(time.Duration(nextInterval) * time.Minute)

	if nextTime.After(end) {
		return nil
	}
	return &nextTime
}
