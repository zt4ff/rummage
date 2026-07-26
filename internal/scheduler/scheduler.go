package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zt4ff/gokue"

	"scaff/internal/db"
	workerpkg "scaff/internal/worker"
)

type Scheduler struct {
	store *db.Store
	queue *gokue.Queue
	wpool *workerpkg.WorkerPool
}

func New(store *db.Store, queue *gokue.Queue, wpool *workerpkg.WorkerPool) *Scheduler {
	return &Scheduler{store: store, queue: queue, wpool: wpool}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Printf("[scheduler] started, checking every minute")
	s.tick(ctx)

	for {
		select {
		case <-ticker.C:
			s.tick(ctx)
		case <-ctx.Done():
			log.Printf("[scheduler] shutting down")
			return
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	topics, err := s.store.TopicsDue(ctx, now)
	if err != nil {
		log.Printf("[scheduler] error fetching due topics: %v", err)
		return
	}
	if len(topics) == 0 {
		return
	}
	log.Printf("[scheduler] tick: found %d due topics", len(topics))

	for _, topic := range topics {
		// Skip if topic already has an active run
		active, err := s.store.HasActiveRun(ctx, topic.ID)
		if err != nil {
			log.Printf("[scheduler] error checking active run for %s: %v", topic.ID, err)
			continue
		}
		if active {
			log.Printf("[scheduler] topic %s (%q) already has active run, skipping", topic.ID, topic.Name)
			continue
		}

		if err := s.runTopic(ctx, topic, now); err != nil {
			log.Printf("[scheduler] error running topic %s (%q): %v", topic.ID, topic.Name, err)
			continue
		}

		nextRun, err := nextCronTime(topic.ScheduleCron, now)
		if err != nil {
			log.Printf("[scheduler] error computing next run for topic %s: %v", topic.ID, err)
			continue
		}
		if err := s.store.UpdateNextRunAt(ctx, topic.ID, nextRun); err != nil {
			log.Printf("[scheduler] error updating next_run_at for topic %s: %v", topic.ID, err)
		}
	}
}

func (s *Scheduler) RunTopicNow(ctx context.Context, topic db.Topic) (string, error) {
	log.Printf("[scheduler] immediate run requested for topic %q (%s)", topic.Name, topic.ID)

	// Check for existing active run first
	active, err := s.store.HasActiveRun(ctx, topic.ID)
	if err != nil {
		return "", fmt.Errorf("check active run: %w", err)
	}
	if active {
		log.Printf("[scheduler] topic %s already has active run, cancelling old runs", topic.ID)
		_, _ = s.store.CancelRunningRuns(ctx, topic.ID)
	}

	run, err := s.store.CreateRun(ctx, topic.ID, time.Now())
	if err != nil {
		return "", err
	}
	if run.ID == "" {
		log.Printf("[scheduler] immediate run: run already exists, returning existing")
		return "", nil
	}

	roles := buildRoles(topic.AgentCount)
	jobs, err := s.store.CreateAgentJobs(ctx, run.ID, roles)
	if err != nil {
		return run.ID, err
	}

	for _, job := range jobs {
		aj := workerpkg.AgentJob{
			JobID:   job.ID,
			RunID:   run.ID,
			Role:    job.Role,
			Topic:   topic,
			Handler: s.wpool.HandleAgentJob,
		}
		if err := s.queue.Submit(ctx, "agent_job", aj); err != nil {
			log.Printf("[scheduler] ERROR submitting immediate job %s: %v", job.ID, err)
			_ = s.store.UpdateAgentJobResult(ctx, job.ID, "failed", nil)
		}
	}

	log.Printf("[scheduler] dispatched %d agent jobs for topic %q (run %s, immediate)", len(jobs), topic.Name, run.ID)
	return run.ID, nil
}

func (s *Scheduler) runTopic(ctx context.Context, topic db.Topic, scheduledFor time.Time) error {
	run, err := s.store.CreateRun(ctx, topic.ID, scheduledFor)
	if err != nil {
		return err
	}
	if run.ID == "" {
		log.Printf("[scheduler] run already exists for topic %s at %s, skipping", topic.ID, scheduledFor)
		return nil
	}

	roles := buildRoles(topic.AgentCount)
	jobs, err := s.store.CreateAgentJobs(ctx, run.ID, roles)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		aj := workerpkg.AgentJob{
			JobID:   job.ID,
			RunID:   run.ID,
			Role:    job.Role,
			Topic:   topic,
			Handler: s.wpool.HandleAgentJob,
		}
		if err := s.queue.Submit(ctx, "agent_job", aj); err != nil {
			log.Printf("[scheduler] ERROR submitting job %s: %v", job.ID, err)
			_ = s.store.UpdateAgentJobResult(ctx, job.ID, "failed", nil)
		}
	}

	log.Printf("[scheduler] dispatched %d agent jobs for topic %q (run %s)", len(jobs), topic.Name, run.ID)
	return nil
}

func buildRoles(agentCount int) []string {
	roles := []string{"general_search", "source_specific"}
	for len(roles) < agentCount {
		roles = append(roles, "general_search")
	}
	return roles[:agentCount]
}

func nextCronTime(cronExpr string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(after), nil
}
