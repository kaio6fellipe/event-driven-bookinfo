CREATE TABLE IF NOT EXISTS dlq_events (
    id                TEXT PRIMARY KEY,
    event_id          TEXT NOT NULL,
    event_type        TEXT NOT NULL,
    event_source      TEXT NOT NULL,
    event_subject     TEXT NOT NULL,
    sensor_name       TEXT NOT NULL,
    failed_trigger    TEXT NOT NULL,
    eventsource_url   TEXT NOT NULL,
    namespace         TEXT NOT NULL,
    original_payload  JSONB NOT NULL,
    payload_hash      TEXT NOT NULL,
    original_headers  JSONB NOT NULL DEFAULT '{}'::jsonb,
    datacontenttype   TEXT NOT NULL DEFAULT 'application/json',
    event_timestamp   TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL,
    retry_count       INT NOT NULL DEFAULT 0,
    max_retries       INT NOT NULL DEFAULT 3,
    last_replayed_at  TIMESTAMPTZ,
    resolved_at       TIMESTAMPTZ,
    resolved_by       TEXT,
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_dlq_events_natural_key
    ON dlq_events (sensor_name, failed_trigger, payload_hash);

CREATE INDEX IF NOT EXISTS idx_dlq_events_status ON dlq_events (status);
CREATE INDEX IF NOT EXISTS idx_dlq_events_event_source ON dlq_events (event_source);
CREATE INDEX IF NOT EXISTS idx_dlq_events_created_at ON dlq_events (created_at);
