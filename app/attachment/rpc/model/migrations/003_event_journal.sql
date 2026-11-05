-- event_journal replaces outbox_records: it is the authority RocketMQ consults
-- when it checks back the outcome of a transactional (half) message. A row
-- exists if and only if the local transaction that published the event
-- committed, so no processed marker or cleanup loop is needed.

CREATE TABLE IF NOT EXISTS event_journal (
    event_uid  TEXT PRIMARY KEY,
    topic      TEXT NOT NULL,
    msg_key    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The outbox poller is gone; drop its table.
DROP TABLE IF EXISTS outbox_records;
