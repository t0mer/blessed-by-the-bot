-- Contacts and their recurring events.
CREATE TABLE contacts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    phone       TEXT    NOT NULL,
    event_date  TEXT    NOT NULL,
    event_type  TEXT    NOT NULL CHECK (event_type IN ('birthday','wedding','anniversary','custom')),
    language    TEXT    NOT NULL,
    relation    TEXT    NOT NULL CHECK (relation IN ('friend','close_friend','family','coworker')),
    importance  INTEGER NOT NULL DEFAULT 3 CHECK (importance BETWEEN 1 AND 5),
    gender      TEXT    NOT NULL CHECK (gender IN ('male','female','other')),
    send_time   TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);
CREATE INDEX idx_contacts_enabled ON contacts(enabled);
CREATE INDEX idx_contacts_event_type ON contacts(event_type);

-- Blessing templates. gender/relation NULL means "any".
CREATE TABLE blessings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type  TEXT    NOT NULL CHECK (event_type IN ('birthday','wedding','anniversary','custom')),
    language    TEXT    NOT NULL,
    gender      TEXT    CHECK (gender IS NULL OR gender IN ('male','female','other')),
    relation    TEXT    CHECK (relation IS NULL OR relation IN ('friend','close_friend','family','coworker')),
    text        TEXT    NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);
CREATE INDEX idx_blessings_lookup ON blessings(event_type, language, enabled);

-- Watched WhatsApp groups.
CREATE TABLE "groups" (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    chat_id    TEXT    NOT NULL UNIQUE,
    language   TEXT    NOT NULL DEFAULT 'he',
    threshold  INTEGER CHECK (threshold IS NULL OR threshold >= 1),
    enabled    INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- Rolling evidence for the group-echo trigger.
CREATE TABLE wish_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id   INTEGER NOT NULL REFERENCES "groups"(id) ON DELETE CASCADE,
    sender_id  TEXT    NOT NULL,
    message_id TEXT    NOT NULL,
    matched    TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    UNIQUE (group_id, message_id)
);
CREATE INDEX idx_wish_events_window ON wish_events(group_id, created_at);

-- Send history; also the idempotency ledger for scheduled sends.
CREATE TABLE send_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    kind        TEXT    NOT NULL CHECK (kind IN ('scheduled','group_echo')),
    contact_id  INTEGER REFERENCES contacts(id) ON DELETE SET NULL,
    group_id    INTEGER REFERENCES "groups"(id) ON DELETE SET NULL,
    blessing_id INTEGER REFERENCES blessings(id) ON DELETE SET NULL,
    provider    TEXT    NOT NULL,
    chat_id     TEXT    NOT NULL,
    status      TEXT    NOT NULL CHECK (status IN ('sent','failed')),
    error       TEXT,
    event_year  INTEGER,
    sent_at     TEXT    NOT NULL
);
-- One successful scheduled send per contact per event year.
CREATE UNIQUE INDEX idx_send_log_scheduled_once
    ON send_log(contact_id, event_year)
    WHERE kind = 'scheduled' AND status = 'sent';
CREATE INDEX idx_send_log_feed ON send_log(sent_at DESC);
CREATE INDEX idx_send_log_group_cooldown ON send_log(group_id, kind, sent_at);

-- Editable wish-detection patterns, seeded by 0002.
CREATE TABLE wish_patterns (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    language TEXT    NOT NULL,
    pattern  TEXT    NOT NULL,
    enabled  INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    UNIQUE (language, pattern)
);

-- Key/value runtime settings. Values are JSON; secrets inside carry the
-- crypto package's "enc:" envelope.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
