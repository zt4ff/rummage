package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Source struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Topic struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ScheduleCron string    `json:"schedule_cron"`
	Sources      []Source  `json:"sources"`
	AgentCount   int       `json:"agent_count"`
	NextRunAt    time.Time `json:"next_run_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type Run struct {
	ID           string    `json:"id"`
	TopicID      string    `json:"topic_id"`
	ScheduledFor time.Time `json:"scheduled_for"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type AgentJob struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Role      string          `json:"role"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type RunResult struct {
	ID           string          `json:"id"`
	RunID        string          `json:"run_id"`
	MergedOutput json.RawMessage `json:"merged_output"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, connString string) (*Store, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("db pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	s := &Store{pool: pool}
	// Run idempotent migrations
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	// Migration 002: ensure run_results.run_id has a UNIQUE constraint
	_, err := s.pool.Exec(ctx, `
		DO $$ BEGIN
			ALTER TABLE run_results ADD CONSTRAINT run_results_run_id_unique UNIQUE (run_id);
		EXCEPTION
			WHEN duplicate_object THEN NULL;
			WHEN duplicate_table THEN NULL;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("migration 002: %w", err)
	}
	_, _ = s.pool.Exec(ctx, `UPDATE runs SET status = 'failed' WHERE status = 'running'`)
	return nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) TopicsDue(ctx context.Context, now time.Time) ([]Topic, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, schedule_cron, sources, agent_count, next_run_at, created_at
		FROM topics
		WHERE next_run_at <= $1
		ORDER BY next_run_at
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		var sourcesJSON json.RawMessage
		if err := rows.Scan(&t.ID, &t.Name, &t.ScheduleCron, &sourcesJSON, &t.AgentCount, &t.NextRunAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sourcesJSON, &t.Sources); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

func (s *Store) CreateRun(ctx context.Context, topicID string, scheduledFor time.Time) (Run, error) {
	var r Run
	err := s.pool.QueryRow(ctx, `
		INSERT INTO runs (topic_id, scheduled_for)
		VALUES ($1, $2)
		ON CONFLICT (topic_id, scheduled_for) DO NOTHING
		RETURNING id, topic_id, scheduled_for, status, created_at
	`, topicID, scheduledFor).Scan(&r.ID, &r.TopicID, &r.ScheduledFor, &r.Status, &r.CreatedAt)
	if err != nil {
		// ON CONFLICT DO NOTHING returns no row — that's not an error for us
		if errors.Is(err, pgx.ErrNoRows) {
			return Run{}, nil
		}
		return Run{}, err
	}
	return r, nil
}

func (s *Store) CreateAgentJobs(ctx context.Context, runID string, roles []string) ([]AgentJob, error) {
	var jobs []AgentJob
	for _, role := range roles {
		var aj AgentJob
		err := s.pool.QueryRow(ctx, `
			INSERT INTO agent_jobs (run_id, role)
			VALUES ($1, $2)
			RETURNING id, run_id, role, status, created_at
		`, runID, role).Scan(&aj.ID, &aj.RunID, &aj.Role, &aj.Status, &aj.CreatedAt)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, aj)
	}
	return jobs, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, runID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE runs SET status = $1 WHERE id = $2`, status, runID)
	return err
}

func (s *Store) UpdateAgentJobResult(ctx context.Context, jobID string, status string, result json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE agent_jobs SET status = $1, result = $2 WHERE id = $3
	`, status, result, jobID)
	return err
}

func (s *Store) GetAgentJob(ctx context.Context, jobID string) (AgentJob, error) {
	var aj AgentJob
	err := s.pool.QueryRow(ctx, `
		SELECT id, run_id, role, status, result, created_at
		FROM agent_jobs WHERE id = $1
	`, jobID).Scan(&aj.ID, &aj.RunID, &aj.Role, &aj.Status, &aj.Result, &aj.CreatedAt)
	return aj, err
}

func (s *Store) AllJobsCompleted(ctx context.Context, runID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_jobs
		WHERE run_id = $1 AND status NOT IN ('completed', 'failed')
	`, runID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// AnyJobsFailed returns true if any agent job in the run has failed.
func (s *Store) AnyJobsFailed(ctx context.Context, runID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_jobs
		WHERE run_id = $1 AND status = 'failed'
	`, runID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) JobResults(ctx context.Context, runID string) ([]AgentJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, role, status, result, created_at
		FROM agent_jobs
		WHERE run_id = $1
		ORDER BY role
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []AgentJob
	for rows.Next() {
		var aj AgentJob
		if err := rows.Scan(&aj.ID, &aj.RunID, &aj.Role, &aj.Status, &aj.Result, &aj.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, aj)
	}
	return jobs, rows.Err()
}

func (s *Store) UpsertRunResult(ctx context.Context, runID string, merged json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO run_results (run_id, merged_output)
		VALUES ($1, $2)
		ON CONFLICT (run_id) DO UPDATE SET merged_output = $2
	`, runID, merged)
	return err
}

func (s *Store) GetTopic(ctx context.Context, id string) (Topic, error) {
	var t Topic
	var sourcesJSON json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, schedule_cron, sources, agent_count, next_run_at, created_at
		FROM topics WHERE id = $1
	`, id).Scan(&t.ID, &t.Name, &t.ScheduleCron, &sourcesJSON, &t.AgentCount, &t.NextRunAt, &t.CreatedAt)
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(sourcesJSON, &t.Sources); err != nil {
		return t, err
	}
	return t, nil
}

func (s *Store) ListTopics(ctx context.Context) ([]Topic, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, schedule_cron, sources, agent_count, next_run_at, created_at
		FROM topics ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		var sourcesJSON json.RawMessage
		if err := rows.Scan(&t.ID, &t.Name, &t.ScheduleCron, &sourcesJSON, &t.AgentCount, &t.NextRunAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sourcesJSON, &t.Sources); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

func (s *Store) TopicRuns(ctx context.Context, topicID string) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, topic_id, scheduled_for, status, created_at
		FROM runs WHERE topic_id = $1
		ORDER BY scheduled_for DESC
	`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.TopicID, &r.ScheduledFor, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) CreateTopic(ctx context.Context, name, cron string, sources []Source, agentCount int) (Topic, error) {
	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return Topic{}, err
	}
	var t Topic
	err = s.pool.QueryRow(ctx, `
		INSERT INTO topics (name, schedule_cron, sources, agent_count)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, schedule_cron, sources, agent_count, next_run_at, created_at
	`, name, cron, sourcesJSON, agentCount).Scan(&t.ID, &t.Name, &t.ScheduleCron, &sourcesJSON, &t.AgentCount, &t.NextRunAt, &t.CreatedAt)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal(sourcesJSON, &t.Sources)
	return t, nil
}

func (s *Store) UpdateNextRunAt(ctx context.Context, topicID string, next time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE topics SET next_run_at = $1 WHERE id = $2`, next, topicID)
	return err
}

func (s *Store) RunWithResult(ctx context.Context, runID string) (Run, *RunResult, []AgentJob, error) {
	r, err := s.RunByID(ctx, runID)
	if err != nil {
		return Run{}, nil, nil, err
	}
	rr, _ := s.RunResultByRunID(ctx, runID)
	jobs, err := s.JobResults(ctx, runID)
	if err != nil {
		return Run{}, nil, nil, err
	}
	return r, rr, jobs, nil
}

func (s *Store) RunByID(ctx context.Context, id string) (Run, error) {
	var r Run
	err := s.pool.QueryRow(ctx, `
		SELECT id, topic_id, scheduled_for, status, created_at
		FROM runs WHERE id = $1
	`, id).Scan(&r.ID, &r.TopicID, &r.ScheduledFor, &r.Status, &r.CreatedAt)
	return r, err
}

func (s *Store) RunResultByRunID(ctx context.Context, runID string) (*RunResult, error) {
	var rr RunResult
	err := s.pool.QueryRow(ctx, `
		SELECT id, run_id, merged_output, created_at
		FROM run_results WHERE run_id = $1
	`, runID).Scan(&rr.ID, &rr.RunID, &rr.MergedOutput, &rr.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

func (s *Store) LastRunForTopic(ctx context.Context, topicID string) (*Run, error) {
	var r Run
	err := s.pool.QueryRow(ctx, `
		SELECT id, topic_id, scheduled_for, status, created_at
		FROM runs WHERE topic_id = $1
		ORDER BY scheduled_for DESC LIMIT 1
	`, topicID).Scan(&r.ID, &r.TopicID, &r.ScheduledFor, &r.Status, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) DeleteTopic(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM topics WHERE id = $1`, id)
	return err
}

func (s *Store) CancelRunningRuns(ctx context.Context, topicID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE runs SET status = 'failed'
		WHERE topic_id = $1 AND status IN ('pending', 'running')
	`, topicID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// HasActiveRun returns true if the topic has a pending or running run.
func (s *Store) HasActiveRun(ctx context.Context, topicID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM runs
		WHERE topic_id = $1 AND status IN ('pending', 'running')
	`, topicID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
