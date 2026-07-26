CREATE TABLE topics (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    schedule_cron TEXT NOT NULL,
    sources     JSONB NOT NULL DEFAULT '[]'::jsonb,
    agent_count INT NOT NULL DEFAULT 3,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id    UUID NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    scheduled_for TIMESTAMPTZ NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic_id, scheduled_for)
);

CREATE TABLE agent_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id      UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('general_search', 'source_specific')),
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    result      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE run_results (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        UUID NOT NULL UNIQUE REFERENCES runs(id) ON DELETE CASCADE,
    merged_output JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_topics_next_run_at ON topics(next_run_at);
CREATE INDEX idx_runs_topic_id ON runs(topic_id);
CREATE INDEX idx_agent_jobs_run_id ON agent_jobs(run_id);
CREATE INDEX idx_run_results_run_id ON run_results(run_id);
