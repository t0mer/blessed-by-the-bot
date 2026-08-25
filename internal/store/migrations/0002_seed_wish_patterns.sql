-- Default wish-detection patterns (spec §2.2). Language 'any' marks
-- language-neutral emoji signals; the echo service matches every enabled
-- pattern regardless of its language label. All of these are editable in the UI.
INSERT INTO wish_patterns (language, pattern, enabled) VALUES
    ('en',  'happy birthday',      1),
    ('en',  'happy bday',          1),
    ('en',  'many happy returns',  1),
    ('en',  'congratulations',     1),
    ('en',  'congrats',            1),
    ('en',  'mazal tov',           1),
    ('en',  'mazel tov',           1),
    ('he',  'מזל טוב',              1),
    ('he',  'המון מזל טוב',          1),
    ('he',  'יום הולדת שמח',         1),
    ('he',  'יומולדת שמח',           1),
    ('he',  'בשעה טובה',            1),
    ('he',  'שיהיה במזל',            1),
    ('any', '🎂',                   1),
    ('any', '🎉',                   1),
    ('any', '🥳',                   1),
    ('any', '🎈',                   1);
