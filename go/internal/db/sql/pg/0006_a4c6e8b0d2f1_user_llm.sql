-- The assistant's LLM provider and API key belong to the account (quest=356), not to the
-- service: the key is stored encrypted (application-level AES-GCM, key outside the
-- database), the hint keeps only its last characters for display.
CREATE TABLE user_llm (
    user_id INTEGER PRIMARY KEY REFERENCES appuser (id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    api_key_enc TEXT NOT NULL DEFAULT '',
    key_hint TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP NOT NULL DEFAULT (now() AT TIME ZONE 'utc')
);
GRANT SELECT, INSERT, UPDATE, DELETE ON user_llm TO quests_app;
