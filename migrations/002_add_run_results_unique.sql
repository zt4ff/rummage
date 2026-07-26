-- Migration 002: Add UNIQUE constraint on run_results.run_id
-- The ON CONFLICT (run_id) in UpsertRunResult needs this.

-- First, delete any duplicate rows (keep newest)
DELETE FROM run_results a
USING run_results b
WHERE a.run_id = b.run_id AND a.created_at < b.created_at;

-- Add the unique constraint
ALTER TABLE run_results ADD CONSTRAINT run_results_run_id_unique UNIQUE (run_id);

-- Reset stuck runs so they can be retried cleanly
UPDATE runs SET status = 'failed' WHERE status = 'running';
