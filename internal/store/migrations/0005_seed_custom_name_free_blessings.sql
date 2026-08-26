-- Close a gap in migration 0003: its own rule is that every event type and
-- language ships at least one template WITHOUT {{name}}, but 'custom' shipped
-- only name-carrying ones in both languages.
--
-- Group echo can post only name-free templates — the bot sees a stream of
-- congratulations, not whose event it is. Echo picks 'birthday' today, so the
-- gap is latent, but spec §7 keeps a hook for mapping a wish pattern to another
-- event type; the first use of that hook against 'custom' would find nothing to
-- send. A separate migration rather than an edit to 0003, which has already
-- been applied wherever the app has run.

INSERT INTO blessings (event_type, language, gender, relation, text, enabled, created_at, updated_at) VALUES
    ('custom', 'he', NULL, NULL, 'מזל טוב! 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ('custom', 'en', NULL, NULL, 'Congratulations! 🎉', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));
