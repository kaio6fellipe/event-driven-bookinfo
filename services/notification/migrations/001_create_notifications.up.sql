CREATE TABLE IF NOT EXISTS notifications (
    id        TEXT PRIMARY KEY,
    recipient TEXT NOT NULL,
    channel   TEXT NOT NULL,
    subject   TEXT NOT NULL,
    body      TEXT NOT NULL,
    status    TEXT NOT NULL DEFAULT 'queued',
    sent_at   TIMESTAMPTZ
);

CREATE INDEX idx_notifications_recipient ON notifications (recipient);
