CREATE TABLE kernel_cron_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cron_id TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'running' CHECK (
        status IN ('running', 'success', 'failed', 'timeout')
    ),
    error_message TEXT,
    attempt INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cron_executions_cron_id ON kernel_cron_executions(cron_id, started_at DESC);
CREATE INDEX idx_cron_executions_status ON kernel_cron_executions(status)
WHERE status = 'running';
