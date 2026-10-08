-- PostgreSQL baseline at alembic 1a2b3c4d5e6f. Generated from the SQLite migration chain.
-- Boolean-ish columns stay INTEGER 0/1, timestamps are naive UTC and VARCHAR(n) is TEXT, matching what SQLite (and so prod data and the Go store) allows.

CREATE TABLE queststep (
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    progress_current INTEGER NOT NULL,
    progress_total INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    id SERIAL PRIMARY KEY,
    quest_id INTEGER,
    check_command TEXT,
    check_interval_seconds INTEGER,
    check_last_run_at TIMESTAMP,
    wait_previous INTEGER DEFAULT 0 NOT NULL,
    run_mode TEXT DEFAULT 'poll' NOT NULL,
    run_status TEXT,
    run_on TEXT
);

CREATE TABLE questtemplatestep (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '' NOT NULL,
    sort_order INTEGER DEFAULT 0 NOT NULL,
    template_id INTEGER,
    check_command TEXT,
    check_interval_seconds INTEGER,
    progress_min INTEGER DEFAULT 1 NOT NULL,
    progress_max INTEGER DEFAULT 1 NOT NULL,
    wait_previous INTEGER DEFAULT 0 NOT NULL,
    run_mode TEXT DEFAULT 'poll' NOT NULL,
    run_on TEXT
);

CREATE TABLE templateemitroll (
    id SERIAL PRIMARY KEY,
    template_id INTEGER NOT NULL,
    period_key TEXT NOT NULL,
    outcome TEXT NOT NULL,
    scheduled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    attempts INTEGER DEFAULT 0 NOT NULL,
    picked_refs TEXT,
    retry_at TIMESTAMP,
    CONSTRAINT uq_template_emit_roll_period UNIQUE (template_id, period_key)
);

CREATE TABLE questcategory (
    id SERIAL PRIMARY KEY,
    slug TEXT NOT NULL,
    label TEXT NOT NULL,
    sort_order INTEGER DEFAULT 0 NOT NULL,
    created_at TIMESTAMP NOT NULL,
    color TEXT DEFAULT '#9a9a9a' NOT NULL,
    UNIQUE (slug)
);

CREATE TABLE questline (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '' NOT NULL,
    category_id INTEGER,
    color TEXT DEFAULT '#9a9a9a' NOT NULL,
    icon TEXT DEFAULT 'document' NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    custom_icon TEXT
);

CREATE TABLE questchangelog (
    id SERIAL PRIMARY KEY,
    at TIMESTAMP NOT NULL,
    kind TEXT NOT NULL,
    quest_id INTEGER,
    title TEXT NOT NULL,
    detail TEXT NOT NULL,
    significance TEXT,
    revision INTEGER,
    comment TEXT,
    entity_type TEXT DEFAULT 'quest' NOT NULL,
    entity_id INTEGER,
    parent_type TEXT,
    parent_id INTEGER
);

CREATE TABLE appuser (
    id SERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_active INTEGER DEFAULT 1 NOT NULL,
    created_at TIMESTAMP NOT NULL,
    last_login_at TIMESTAMP
);

CREATE TABLE usersession (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    last_seen_at TIMESTAMP,
    user_agent TEXT DEFAULT '' NOT NULL
);

CREATE TABLE apitoken (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE TABLE backuplog (
    id SERIAL PRIMARY KEY,
    filename TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL,
    webdav_uploaded INTEGER DEFAULT 0 NOT NULL,
    error TEXT,
    remote_uploaded INTEGER DEFAULT 0 NOT NULL,
    attachments_filename TEXT,
    attachments_size_bytes INTEGER
);

CREATE TABLE attachment_revision (
    id SERIAL PRIMARY KEY,
    attachment_id INTEGER NOT NULL,
    revision INTEGER NOT NULL,
    filename TEXT NOT NULL,
    webdav_path TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0 NOT NULL,
    content_type_declared TEXT DEFAULT '' NOT NULL,
    content_type_detected TEXT DEFAULT '' NOT NULL,
    comment TEXT DEFAULT '' NOT NULL,
    uploaded_at TIMESTAMP NOT NULL,
    scan_status TEXT DEFAULT 'pending' NOT NULL,
    scanned_at TIMESTAMP,
    CONSTRAINT uq_attachment_revision UNIQUE (attachment_id, revision),
    CONSTRAINT uq_attachment_revision_webdav_path UNIQUE (webdav_path)
);

CREATE TABLE tag (
    id SERIAL PRIMARY KEY,
    slug TEXT NOT NULL,
    label TEXT NOT NULL,
    color TEXT DEFAULT '#9a9a9a' NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE (slug)
);

CREATE TABLE quest_tag (
    quest_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (quest_id, tag_id)
);

CREATE TABLE template_tag (
    template_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (template_id, tag_id)
);

CREATE TABLE templateemitattempt (
    id SERIAL PRIMARY KEY,
    template_id INTEGER NOT NULL,
    period_key TEXT NOT NULL,
    at TIMESTAMP NOT NULL,
    attempt INTEGER DEFAULT 1 NOT NULL,
    status TEXT NOT NULL,
    duration_ms INTEGER DEFAULT 0 NOT NULL,
    items INTEGER DEFAULT 0 NOT NULL,
    picked INTEGER DEFAULT 0 NOT NULL,
    picked_refs TEXT,
    message TEXT
);

CREATE TABLE secret (
    id SERIAL PRIMARY KEY,
    questline_id INTEGER,
    template_id INTEGER,
    quest_id INTEGER,
    step_id INTEGER,
    "key" TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT ck_secret_one_owner CHECK ((questline_id IS NOT NULL)::int + (template_id IS NOT NULL)::int + (quest_id IS NOT NULL)::int + (step_id IS NOT NULL)::int = 1)
);

CREATE TABLE note (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '' NOT NULL,
    pinned INTEGER DEFAULT 0 NOT NULL,
    sort_order INTEGER DEFAULT 0 NOT NULL,
    parent_id INTEGER,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    color TEXT DEFAULT '#9a9a9a' NOT NULL,
    icon TEXT DEFAULT 'document' NOT NULL,
    custom_icon TEXT,
    is_readme INTEGER DEFAULT 0 NOT NULL,
    is_private INTEGER DEFAULT 0 NOT NULL,
    is_category INTEGER DEFAULT 0 NOT NULL
);

CREATE TABLE attachment (
    id SERIAL PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_id INTEGER NOT NULL,
    filename TEXT NOT NULL,
    webdav_path TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0 NOT NULL,
    content_type_declared TEXT DEFAULT '' NOT NULL,
    content_type_detected TEXT DEFAULT '' NOT NULL,
    comment TEXT DEFAULT '' NOT NULL,
    uploaded_at TIMESTAMP NOT NULL,
    scan_status TEXT DEFAULT 'pending' NOT NULL,
    scanned_at TIMESTAMP,
    current_revision INTEGER DEFAULT 1 NOT NULL,
    hidden INTEGER DEFAULT 0 NOT NULL
);

CREATE TABLE questtemplate (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '' NOT NULL,
    pinned INTEGER DEFAULT 0 NOT NULL,
    sort_order INTEGER DEFAULT 0 NOT NULL,
    duration_seconds INTEGER,
    freq TEXT DEFAULT 'daily' NOT NULL,
    weekdays TEXT DEFAULT '0,
    1,
    2,
    3,
    4,
    5,
    6' NOT NULL,
    enabled INTEGER DEFAULT 1 NOT NULL,
    timezone TEXT DEFAULT 'Europe/Moscow' NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deadline_time TEXT,
    significance TEXT DEFAULT 'common' NOT NULL,
    emit_mode TEXT DEFAULT 'fixed' NOT NULL,
    emit_chance DOUBLE PRECISION DEFAULT 1.0 NOT NULL,
    emit_window_start TEXT,
    emit_window_end TEXT,
    category_id INTEGER,
    questline_id INTEGER,
    automated INTEGER DEFAULT 0 NOT NULL,
    emit_pool_command TEXT,
    emit_pool_pick INTEGER DEFAULT 1 NOT NULL,
    emit_limits TEXT,
    emit_run_on TEXT
);

CREATE TABLE quest (
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL,
    pinned INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    deadline_at TIMESTAMP,
    duration_seconds INTEGER,
    template_id INTEGER,
    period_key TEXT,
    significance TEXT DEFAULT 'common' NOT NULL,
    category_id INTEGER,
    questline_id INTEGER,
    automated INTEGER DEFAULT 0 NOT NULL,
    source TEXT,
    CONSTRAINT uq_quest_template_period UNIQUE (template_id, period_key)
);

CREATE TABLE graphdraft_op (
    id SERIAL PRIMARY KEY,
    kind TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE graphdraft_state (
    id SERIAL PRIMARY KEY,
    roots TEXT DEFAULT '[]' NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE runner (
    id SERIAL PRIMARY KEY,
    label TEXT NOT NULL,
    reg_hash TEXT,
    reg_expires_at TIMESTAMP,
    token_hash TEXT,
    hostname TEXT,
    version TEXT,
    last_seen_at TIMESTAMP,
    revoked INTEGER DEFAULT 0 NOT NULL,
    created_at TIMESTAMP NOT NULL,
    UNIQUE (label)
);

CREATE TABLE runner_job (
    id SERIAL PRIMARY KEY,
    runner_label TEXT NOT NULL,
    kind TEXT NOT NULL,
    quest_id INTEGER,
    step_id INTEGER,
    template_id INTEGER,
    title TEXT DEFAULT '' NOT NULL,
    dedupe_key TEXT,
    command TEXT NOT NULL,
    state TEXT DEFAULT 'queued' NOT NULL,
    attempt_no INTEGER DEFAULT 1 NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    lease_id TEXT,
    lease_until TIMESTAMP,
    valid_until TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    assigned_at TIMESTAMP,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    exit_code INTEGER,
    timed_out INTEGER DEFAULT 0 NOT NULL,
    stdout TEXT DEFAULT '' NOT NULL,
    stderr TEXT DEFAULT '' NOT NULL,
    progress_current INTEGER,
    progress_total INTEGER,
    log_tail TEXT DEFAULT '' NOT NULL,
    reason TEXT DEFAULT '' NOT NULL,
    cancel INTEGER DEFAULT 0 NOT NULL
);

ALTER TABLE queststep ADD CONSTRAINT fk_queststep_quest_id FOREIGN KEY (quest_id) REFERENCES quest (id);
ALTER TABLE questtemplatestep ADD CONSTRAINT fk_questtemplatestep_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id);
ALTER TABLE templateemitroll ADD CONSTRAINT fk_templateemitroll_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id);
ALTER TABLE questline ADD CONSTRAINT fk_questline_category_id FOREIGN KEY (category_id) REFERENCES questcategory (id);
ALTER TABLE questchangelog ADD CONSTRAINT fk_questchangelog_quest_id FOREIGN KEY (quest_id) REFERENCES quest (id) ON DELETE SET NULL;
ALTER TABLE usersession ADD CONSTRAINT fk_usersession_user_id FOREIGN KEY (user_id) REFERENCES appuser (id) ON DELETE CASCADE;
ALTER TABLE apitoken ADD CONSTRAINT fk_apitoken_user_id FOREIGN KEY (user_id) REFERENCES appuser (id) ON DELETE CASCADE;
ALTER TABLE attachment_revision ADD CONSTRAINT fk_attachment_revision_attachment_id FOREIGN KEY (attachment_id) REFERENCES attachment (id) ON DELETE CASCADE;
ALTER TABLE quest_tag ADD CONSTRAINT fk_quest_tag_quest_id FOREIGN KEY (quest_id) REFERENCES quest (id) ON DELETE CASCADE;
ALTER TABLE quest_tag ADD CONSTRAINT fk_quest_tag_tag_id FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE;
ALTER TABLE template_tag ADD CONSTRAINT fk_template_tag_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id) ON DELETE CASCADE;
ALTER TABLE template_tag ADD CONSTRAINT fk_template_tag_tag_id FOREIGN KEY (tag_id) REFERENCES tag (id) ON DELETE CASCADE;
ALTER TABLE templateemitattempt ADD CONSTRAINT fk_templateemitattempt_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id) ON DELETE CASCADE;
ALTER TABLE secret ADD CONSTRAINT fk_secret_questline_id FOREIGN KEY (questline_id) REFERENCES questline (id) ON DELETE CASCADE;
ALTER TABLE secret ADD CONSTRAINT fk_secret_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id) ON DELETE CASCADE;
ALTER TABLE secret ADD CONSTRAINT fk_secret_quest_id FOREIGN KEY (quest_id) REFERENCES quest (id) ON DELETE CASCADE;
ALTER TABLE secret ADD CONSTRAINT fk_secret_step_id FOREIGN KEY (step_id) REFERENCES queststep (id) ON DELETE CASCADE;
ALTER TABLE questtemplate ADD CONSTRAINT fk_questtemplate_category_id FOREIGN KEY (category_id) REFERENCES questcategory (id);
ALTER TABLE questtemplate ADD CONSTRAINT fk_questtemplate_questline_id FOREIGN KEY (questline_id) REFERENCES questline (id);
ALTER TABLE quest ADD CONSTRAINT fk_quest_template_id FOREIGN KEY (template_id) REFERENCES questtemplate (id);
ALTER TABLE quest ADD CONSTRAINT fk_quest_questline_id FOREIGN KEY (questline_id) REFERENCES questline (id);
ALTER TABLE quest ADD CONSTRAINT fk_quest_category_id FOREIGN KEY (category_id) REFERENCES questcategory (id);

CREATE INDEX ix_queststep_quest_id ON queststep (quest_id);
CREATE INDEX ix_questtemplatestep_template_id ON questtemplatestep (template_id);
CREATE INDEX ix_templateemitroll_template_id ON templateemitroll (template_id);
CREATE INDEX ix_templateemitroll_period_key ON templateemitroll (period_key);
CREATE INDEX ix_questcategory_slug ON questcategory (slug);
CREATE INDEX ix_questline_category_id ON questline (category_id);
CREATE INDEX ix_questchangelog_at ON questchangelog (at);
CREATE INDEX ix_questchangelog_kind ON questchangelog (kind);
CREATE INDEX ix_questchangelog_quest_id ON questchangelog (quest_id);
CREATE INDEX ix_questchangelog_revision ON questchangelog (revision);
CREATE UNIQUE INDEX ix_appuser_username ON appuser (username);
CREATE UNIQUE INDEX ix_usersession_token_hash ON usersession (token_hash);
CREATE INDEX ix_usersession_user_id ON usersession (user_id);
CREATE UNIQUE INDEX ix_apitoken_token_hash ON apitoken (token_hash);
CREATE INDEX ix_apitoken_user_id ON apitoken (user_id);
CREATE INDEX ix_backuplog_created_at ON backuplog (created_at);
CREATE INDEX ix_attachment_revision_attachment_id ON attachment_revision (attachment_id);
CREATE INDEX ix_tag_slug ON tag (slug);
CREATE INDEX ix_quest_tag_tag_id ON quest_tag (tag_id);
CREATE INDEX ix_template_tag_tag_id ON template_tag (tag_id);
CREATE INDEX ix_templateemitattempt_template_id_id ON templateemitattempt (template_id, id);
CREATE UNIQUE INDEX ux_secret_questline_id_key ON secret (questline_id, "key") WHERE questline_id IS NOT NULL;
CREATE UNIQUE INDEX ux_secret_template_id_key ON secret (template_id, "key") WHERE template_id IS NOT NULL;
CREATE UNIQUE INDEX ux_secret_quest_id_key ON secret (quest_id, "key") WHERE quest_id IS NOT NULL;
CREATE UNIQUE INDEX ux_secret_step_id_key ON secret (step_id, "key") WHERE step_id IS NOT NULL;
CREATE INDEX ix_note_pinned ON note (pinned);
CREATE INDEX ix_note_parent_id ON note (parent_id);
CREATE INDEX ix_attachment_owner_id ON attachment (owner_id);
CREATE INDEX ix_attachment_owner_type ON attachment (owner_type);
CREATE UNIQUE INDEX ix_attachment_webdav_path ON attachment (webdav_path);
CREATE INDEX ix_attachment_owner ON attachment (owner_type, owner_id);
CREATE INDEX ix_questtemplate_questline_id ON questtemplate (questline_id);
CREATE INDEX ix_questtemplate_category_id ON questtemplate (category_id);
CREATE INDEX ix_quest_template_id ON quest (template_id);
CREATE INDEX ix_quest_category_id ON quest (category_id);
CREATE INDEX ix_quest_questline_id ON quest (questline_id);
CREATE INDEX ix_quest_period_key ON quest (period_key);
CREATE INDEX ix_questchangelog_entity ON questchangelog (entity_type, entity_id);
CREATE INDEX ix_questchangelog_parent ON questchangelog (parent_type, parent_id);
CREATE INDEX ix_runner_job_label_state ON runner_job (runner_label, state);
CREATE UNIQUE INDEX ux_runner_job_open_dedupe ON runner_job (dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('queued','assigned','running');
