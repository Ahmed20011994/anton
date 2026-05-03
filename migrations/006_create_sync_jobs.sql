CREATE TABLE sync_jobs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_type     TEXT NOT NULL,
    status          TEXT NOT NULL,
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    heartbeat_at    TIMESTAMPTZ,
    worker_id       TEXT,
    since_used      TIMESTAMPTZ,
    created_count   INT NOT NULL DEFAULT 0,
    updated_count   INT NOT NULL DEFAULT 0,
    fetch_errors    INT NOT NULL DEFAULT 0,
    error           TEXT
);

CREATE INDEX idx_sync_jobs_queue   ON sync_jobs(status, requested_at);
CREATE INDEX idx_sync_jobs_tenant  ON sync_jobs(tenant_id, source_type, status);
CREATE INDEX idx_sync_jobs_running ON sync_jobs(status, heartbeat_at)
    WHERE status = 'running';
