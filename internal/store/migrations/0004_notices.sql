-- Application notices: conditions the operator should see in the UI but that
-- are not failures worth refusing work over.
--
-- The motivating case is spec §6: when a contact's language has no template the
-- scheduler falls back to English and sends anyway. A log line is invisible to
-- someone using the web UI, and the condition recurs every year until a template
-- is added, so it needs somewhere durable to live.
--
-- `key` is UNIQUE and is the notice's identity: raising the same condition twice
-- bumps the counter and the timestamp rather than appending a duplicate, so a
-- daily-recurring problem cannot flood the list.
CREATE TABLE notices (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    key           TEXT    NOT NULL UNIQUE,
    level         TEXT    NOT NULL CHECK (level IN ('info','warning','error')),
    code          TEXT    NOT NULL,
    message       TEXT    NOT NULL,
    detail        TEXT,
    occurrences   INTEGER NOT NULL DEFAULT 1,
    first_seen_at TEXT    NOT NULL,
    last_seen_at  TEXT    NOT NULL,
    dismissed_at  TEXT
);

-- The UI asks for undismissed notices, newest first.
CREATE INDEX idx_notices_active ON notices(dismissed_at, last_seen_at DESC);
