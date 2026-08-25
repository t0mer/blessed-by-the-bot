-- Starter blessing templates so a fresh install can send on day one. Without
-- these the scheduler finds a due contact, finds no template, and logs an error
-- the user has no reason to expect. All of them are editable or deletable in
-- the UI; this is a starting point, not a fixed set.
--
-- Two rules drive the content:
--   * Hebrew is grammatically gendered, so birthday and anniversary greetings
--     come in male and female forms. The selector prefers an exact gender match
--     and excludes a mismatched one outright.
--   * Timestamps use strftime, not datetime('now'): the store scans them as
--     RFC3339Nano and SQLite's default "YYYY-MM-DD HH:MM:SS" does not parse.
--   * Every event type and language also gets at least one template WITHOUT
--     {{name}}. Group echo can only use name-free templates — the bot does not
--     know whose birthday a group is celebrating — so omitting them would leave
--     the echo engine with nothing to send.

INSERT INTO blessings (event_type, language, gender, relation, text, enabled, created_at, updated_at) VALUES
    -- Hebrew birthday, gendered.
    ('birthday', 'he', 'male',   NULL, 'יום הולדת שמח {{name}}! שתזכה לשנה מלאה בבריאות, אושר והצלחה 🎂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'he', 'female', NULL, 'יום הולדת שמח {{name}}! שתזכי לשנה מלאה בבריאות, אושר והצלחה 🎂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'he', 'male',   NULL, 'מזל טוב {{name}}! שתמשיך לחייך ולשמח את כל מי שסביבך 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'he', 'female', NULL, 'מזל טוב {{name}}! שתמשיכי לחייך ולשמח את כל מי שסביבך 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    -- Hebrew birthday, name-free: the only kind group echo can use.
    ('birthday', 'he', NULL, NULL, 'מזל טוב ויום הולדת שמח! 🎂🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'he', NULL, NULL, 'מצטרפים לברכות! יום הולדת שמח ושנה נפלאה 🥳', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

    -- English birthday.
    ('birthday', 'en', NULL, NULL, 'Happy birthday {{name}}! Wishing you a wonderful year ahead 🎂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'en', NULL, NULL, 'Many happy returns, {{name}}! Have a brilliant day 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('birthday', 'en', NULL, NULL, 'Happy birthday! 🎂🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

    -- Weddings are addressed to a couple, so they stay gender-neutral.
    ('wedding', 'he', NULL, NULL, 'מזל טוב {{name}}! שתבנו בית נאמן ומאושר יחד ❤️', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('wedding', 'he', NULL, NULL, 'מזל טוב ובשעה טובה! איחולים חמים לזוג המאושר ❤️', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('wedding', 'en', NULL, NULL, 'Congratulations {{name}}! Wishing you both a lifetime of happiness ❤️', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('wedding', 'en', NULL, NULL, 'Congratulations to the happy couple! ❤️', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

    -- Anniversaries, gendered in Hebrew where the greeting addresses one person.
    ('anniversary', 'he', 'male',   NULL, 'מזל טוב {{name}}! שתמשיכו לחגוג יחד עוד שנים רבות 🥂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('anniversary', 'he', 'female', NULL, 'מזל טוב {{name}}! שתמשיכו לחגוג יחד עוד שנים רבות 🥂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('anniversary', 'he', NULL, NULL, 'מזל טוב לרגל יום הנישואין! 🥂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('anniversary', 'en', NULL, NULL, 'Happy anniversary {{name}}! Here is to many more years together 🥂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('anniversary', 'en', NULL, NULL, 'Happy anniversary! 🥂', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

    -- 'custom' ships one of each language so a user-defined event type is not a
    -- dead end before they write their own.
    ('custom', 'he', NULL, NULL, 'מזל טוב {{name}}! 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('custom', 'en', NULL, NULL, 'Congratulations {{name}}! 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));
