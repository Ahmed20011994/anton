CREATE TABLE work_items (
    id                       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id                UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_id                TEXT NOT NULL,
    source_type              TEXT NOT NULL,
    item_type                TEXT,
    title                    TEXT,
    description              TEXT,
    status                   TEXT,
    status_category          TEXT,
    priority                 TEXT,
    assignees                TEXT[] NOT NULL DEFAULT '{}',
    created_at               TIMESTAMPTZ,
    updated_at               TIMESTAMPTZ,
    closed_at                TIMESTAMPTZ,
    time_in_current_status   INTERVAL,
    cycle_time               INTERVAL,
    reopen_count             INT NOT NULL DEFAULT 0,
    comments                 JSONB NOT NULL DEFAULT '[]'::jsonb,
    linked_customer_signals  TEXT[] NOT NULL DEFAULT '{}',
    version                  TEXT,
    raw_payload              JSONB,
    content_hash             TEXT,
    ingested_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_type, source_id)
);

CREATE INDEX idx_wi_tenant            ON work_items(tenant_id);
CREATE INDEX idx_wi_tenant_updated    ON work_items(tenant_id, updated_at DESC);
CREATE INDEX idx_wi_tenant_status_cat ON work_items(tenant_id, status_category);
CREATE INDEX idx_wi_tenant_type       ON work_items(tenant_id, item_type);
