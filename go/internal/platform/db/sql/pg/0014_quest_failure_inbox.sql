-- Failed quests that the journal should surface until the user dismisses them.
-- Membership is this row with dismissed_at IS NULL; historical failures without a
-- row stay out of the inbox so an upgrade does not dump every old failed quest.
CREATE TABLE quest_failure_inbox (
    quest_id INTEGER PRIMARY KEY REFERENCES quest(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL,
    dismissed_at TIMESTAMP
);
