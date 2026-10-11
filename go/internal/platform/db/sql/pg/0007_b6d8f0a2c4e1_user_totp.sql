-- Two-factor sign-in (quest=359): a TOTP secret per account (sealed with the server key, never
-- stored in the clear) and one-time recovery codes (only their hashes are kept).
-- `enabled = 0` is a secret handed out during enrolment but not yet confirmed with a code.
-- `last_step` is the newest 30-second step already accepted: a code is good once.
CREATE TABLE user_totp (
    user_id INTEGER PRIMARY KEY REFERENCES appuser (id) ON DELETE CASCADE,
    secret_enc TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    last_step BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT (now() AT TIME ZONE 'utc')
);
CREATE TABLE user_recovery_code (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES appuser (id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT (now() AT TIME ZONE 'utc')
);
CREATE INDEX ix_user_recovery_code_user_id ON user_recovery_code (user_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON user_totp, user_recovery_code TO quests_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO quests_app;
