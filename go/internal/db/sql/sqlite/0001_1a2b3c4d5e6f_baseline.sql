CREATE TABLE alembic_version (
	version_num VARCHAR(32) NOT NULL, 
	CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num)
);
INSERT INTO alembic_version VALUES('1a2b3c4d5e6f');
CREATE TABLE queststep (
	title VARCHAR(200) NOT NULL, 
	description VARCHAR NOT NULL, 
	progress_current INTEGER NOT NULL, 
	progress_total INTEGER NOT NULL, 
	sort_order INTEGER NOT NULL, 
	id INTEGER NOT NULL, 
	quest_id INTEGER, check_command VARCHAR(2000), check_interval_seconds INTEGER, check_last_run_at DATETIME, wait_previous INTEGER DEFAULT '0' NOT NULL, run_mode VARCHAR(16) DEFAULT 'poll' NOT NULL, run_status VARCHAR(16), run_on VARCHAR(64), 
	PRIMARY KEY (id), 
	FOREIGN KEY(quest_id) REFERENCES quest (id)
);
CREATE TABLE IF NOT EXISTS "questtemplatestep" (
	id INTEGER NOT NULL, 
	title VARCHAR(200) NOT NULL, 
	description VARCHAR DEFAULT ('') NOT NULL, 
	sort_order INTEGER DEFAULT '0' NOT NULL, 
	template_id INTEGER, 
	check_command VARCHAR(2000), 
	check_interval_seconds INTEGER, 
	progress_min INTEGER DEFAULT '1' NOT NULL, 
	progress_max INTEGER DEFAULT '1' NOT NULL, wait_previous INTEGER DEFAULT '0' NOT NULL, run_mode VARCHAR(16) DEFAULT 'poll' NOT NULL, run_on VARCHAR(64), 
	PRIMARY KEY (id), 
	FOREIGN KEY(template_id) REFERENCES questtemplate (id)
);
CREATE TABLE templateemitroll (
	id INTEGER NOT NULL, 
	template_id INTEGER NOT NULL, 
	period_key VARCHAR(32) NOT NULL, 
	outcome VARCHAR NOT NULL, 
	scheduled_at DATETIME, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, attempts INTEGER DEFAULT '0' NOT NULL, picked_refs VARCHAR, retry_at DATETIME, 
	PRIMARY KEY (id), 
	FOREIGN KEY(template_id) REFERENCES questtemplate (id), 
	CONSTRAINT uq_template_emit_roll_period UNIQUE (template_id, period_key)
);
CREATE TABLE questcategory (
	id INTEGER NOT NULL, 
	slug VARCHAR(32) NOT NULL, 
	label VARCHAR(64) NOT NULL, 
	sort_order INTEGER DEFAULT '0' NOT NULL, 
	created_at DATETIME NOT NULL, color VARCHAR(16) DEFAULT '#9a9a9a' NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (slug)
);
INSERT INTO questcategory VALUES(1,'work','Работа',10,'2026-10-10 17:49:03','#5a8a9a');
INSERT INTO questcategory VALUES(2,'routine','Рутина',20,'2026-10-10 17:49:03','#8a8578');
INSERT INTO questcategory VALUES(3,'health','Здоровье',30,'2026-10-10 17:49:03','#7a9e3a');
INSERT INTO questcategory VALUES(4,'study','Учёба',40,'2026-10-10 17:49:03','#6a7ab8');
INSERT INTO questcategory VALUES(5,'fun','Развлечения',50,'2026-10-10 17:49:03','#c47a20');
CREATE TABLE questline (
	id INTEGER NOT NULL, 
	title VARCHAR(200) NOT NULL, 
	description VARCHAR DEFAULT '' NOT NULL, 
	category_id INTEGER, 
	color VARCHAR(16) DEFAULT '#9a9a9a' NOT NULL, 
	icon VARCHAR(32) DEFAULT 'document' NOT NULL, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, custom_icon VARCHAR(64), 
	PRIMARY KEY (id), 
	FOREIGN KEY(category_id) REFERENCES questcategory (id)
);
CREATE TABLE questchangelog (
	id INTEGER NOT NULL, 
	at DATETIME NOT NULL, 
	kind VARCHAR(32) NOT NULL, 
	quest_id INTEGER, 
	title VARCHAR(200) NOT NULL, 
	detail VARCHAR(500) NOT NULL, 
	significance VARCHAR(16), 
	revision INTEGER, comment TEXT, entity_type VARCHAR(16) DEFAULT 'quest' NOT NULL, entity_id INTEGER, parent_type VARCHAR(16), parent_id INTEGER, 
	PRIMARY KEY (id), 
	FOREIGN KEY(quest_id) REFERENCES quest (id) ON DELETE SET NULL
);
CREATE TABLE appuser (
	id INTEGER NOT NULL, 
	username VARCHAR(64) NOT NULL, 
	password_hash VARCHAR(256) NOT NULL, 
	is_active BOOLEAN DEFAULT 1 NOT NULL, 
	created_at DATETIME NOT NULL, 
	last_login_at DATETIME, 
	PRIMARY KEY (id)
);
CREATE TABLE usersession (
	id INTEGER NOT NULL, 
	user_id INTEGER NOT NULL, 
	token_hash VARCHAR(64) NOT NULL, 
	created_at DATETIME NOT NULL, 
	expires_at DATETIME NOT NULL, 
	last_seen_at DATETIME, 
	user_agent VARCHAR(256) DEFAULT '' NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES appuser (id) ON DELETE CASCADE
);
CREATE TABLE apitoken (
	id INTEGER NOT NULL, 
	user_id INTEGER NOT NULL, 
	name VARCHAR(64) NOT NULL, 
	token_hash VARCHAR(64) NOT NULL, 
	created_at DATETIME NOT NULL, 
	last_used_at DATETIME, 
	revoked_at DATETIME, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES appuser (id) ON DELETE CASCADE
);
CREATE TABLE backuplog (
	id INTEGER NOT NULL, 
	filename VARCHAR(255) NOT NULL, 
	size_bytes INTEGER NOT NULL, 
	created_at DATETIME NOT NULL, 
	webdav_uploaded BOOLEAN DEFAULT '0' NOT NULL, 
	error VARCHAR, remote_uploaded BOOLEAN DEFAULT '0' NOT NULL, attachments_filename VARCHAR(255), attachments_size_bytes INTEGER, 
	PRIMARY KEY (id)
);
CREATE TABLE attachment_revision (
	id INTEGER NOT NULL, 
	attachment_id INTEGER NOT NULL, 
	revision INTEGER NOT NULL, 
	filename VARCHAR(255) NOT NULL, 
	webdav_path VARCHAR(512) NOT NULL, 
	size_bytes INTEGER DEFAULT '0' NOT NULL, 
	content_type_declared VARCHAR(128) DEFAULT '' NOT NULL, 
	content_type_detected VARCHAR(128) DEFAULT '' NOT NULL, 
	comment VARCHAR(500) DEFAULT '' NOT NULL, 
	uploaded_at DATETIME NOT NULL, 
	scan_status VARCHAR(16) DEFAULT 'pending' NOT NULL, 
	scanned_at DATETIME, 
	PRIMARY KEY (id), 
	FOREIGN KEY(attachment_id) REFERENCES attachment (id) ON DELETE CASCADE, 
	CONSTRAINT uq_attachment_revision UNIQUE (attachment_id, revision), 
	CONSTRAINT uq_attachment_revision_webdav_path UNIQUE (webdav_path)
);
CREATE TABLE tag (
	id INTEGER NOT NULL, 
	slug VARCHAR(64) NOT NULL, 
	label VARCHAR(64) NOT NULL, 
	color VARCHAR(16) DEFAULT '#9a9a9a' NOT NULL, 
	created_at DATETIME NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (slug)
);
CREATE TABLE quest_tag (
	quest_id INTEGER NOT NULL, 
	tag_id INTEGER NOT NULL, 
	PRIMARY KEY (quest_id, tag_id), 
	FOREIGN KEY(quest_id) REFERENCES quest (id) ON DELETE CASCADE, 
	FOREIGN KEY(tag_id) REFERENCES tag (id) ON DELETE CASCADE
);
CREATE TABLE template_tag (
	template_id INTEGER NOT NULL, 
	tag_id INTEGER NOT NULL, 
	PRIMARY KEY (template_id, tag_id), 
	FOREIGN KEY(template_id) REFERENCES questtemplate (id) ON DELETE CASCADE, 
	FOREIGN KEY(tag_id) REFERENCES tag (id) ON DELETE CASCADE
);
CREATE TABLE templateemitattempt (
	id INTEGER NOT NULL, 
	template_id INTEGER NOT NULL, 
	period_key VARCHAR(32) NOT NULL, 
	at DATETIME NOT NULL, 
	attempt INTEGER DEFAULT '1' NOT NULL, 
	status VARCHAR(16) NOT NULL, 
	duration_ms INTEGER DEFAULT '0' NOT NULL, 
	items INTEGER DEFAULT '0' NOT NULL, 
	picked INTEGER DEFAULT '0' NOT NULL, 
	picked_refs VARCHAR, 
	message VARCHAR, 
	PRIMARY KEY (id), 
	FOREIGN KEY(template_id) REFERENCES questtemplate (id) ON DELETE CASCADE
);
CREATE TABLE secret (
	id INTEGER NOT NULL, 
	questline_id INTEGER, 
	template_id INTEGER, 
	quest_id INTEGER, 
	step_id INTEGER, 
	"key" VARCHAR(128) NOT NULL, 
	value TEXT NOT NULL, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, 
	PRIMARY KEY (id), 
	CONSTRAINT ck_secret_one_owner CHECK ((questline_id IS NOT NULL) + (template_id IS NOT NULL) + (quest_id IS NOT NULL) + (step_id IS NOT NULL) = 1), 
	FOREIGN KEY(questline_id) REFERENCES questline (id) ON DELETE CASCADE, 
	FOREIGN KEY(template_id) REFERENCES questtemplate (id) ON DELETE CASCADE, 
	FOREIGN KEY(quest_id) REFERENCES quest (id) ON DELETE CASCADE, 
	FOREIGN KEY(step_id) REFERENCES queststep (id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS "note" (
	id INTEGER NOT NULL, 
	title VARCHAR NOT NULL, 
	description TEXT DEFAULT ('') NOT NULL, 
	pinned INTEGER DEFAULT '0' NOT NULL, 
	sort_order INTEGER DEFAULT '0' NOT NULL, 
	parent_id INTEGER, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, 
	color VARCHAR(16) DEFAULT '#9a9a9a' NOT NULL, 
	icon VARCHAR(32) DEFAULT 'document' NOT NULL, 
	custom_icon VARCHAR(128), 
	is_readme BOOLEAN DEFAULT 0 NOT NULL, 
	is_private BOOLEAN DEFAULT 0 NOT NULL, 
	is_category BOOLEAN DEFAULT 0 NOT NULL, 
	PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS "attachment" (
	id INTEGER NOT NULL, 
	owner_type VARCHAR(16) NOT NULL, 
	owner_id INTEGER NOT NULL, 
	filename VARCHAR(255) NOT NULL, 
	webdav_path VARCHAR(512) NOT NULL, 
	size_bytes INTEGER DEFAULT '0' NOT NULL, 
	content_type_declared VARCHAR(128) DEFAULT ('') NOT NULL, 
	content_type_detected VARCHAR(128) DEFAULT ('') NOT NULL, 
	comment VARCHAR(500) DEFAULT ('') NOT NULL, 
	uploaded_at DATETIME NOT NULL, 
	scan_status VARCHAR(16) DEFAULT 'pending' NOT NULL, 
	scanned_at DATETIME, 
	current_revision INTEGER DEFAULT '1' NOT NULL, 
	hidden BOOLEAN DEFAULT 0 NOT NULL, 
	PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS "questtemplate" (
	id INTEGER NOT NULL, 
	title VARCHAR(200) NOT NULL, 
	description VARCHAR DEFAULT ('') NOT NULL, 
	pinned BOOLEAN DEFAULT 0 NOT NULL, 
	sort_order INTEGER DEFAULT '0' NOT NULL, 
	duration_seconds INTEGER, 
	freq VARCHAR DEFAULT 'daily' NOT NULL, 
	weekdays VARCHAR(32) DEFAULT '0,1,2,3,4,5,6' NOT NULL, 
	enabled BOOLEAN DEFAULT 1 NOT NULL, 
	timezone VARCHAR(64) DEFAULT 'Europe/Moscow' NOT NULL, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, 
	deadline_time VARCHAR(8), 
	significance VARCHAR DEFAULT 'common' NOT NULL, 
	emit_mode VARCHAR DEFAULT 'fixed' NOT NULL, 
	emit_chance FLOAT DEFAULT '1.0' NOT NULL, 
	emit_window_start VARCHAR(8), 
	emit_window_end VARCHAR(8), 
	category_id INTEGER, 
	questline_id INTEGER, 
	automated INTEGER DEFAULT '0' NOT NULL, 
	emit_pool_command VARCHAR(2000), 
	emit_pool_pick INTEGER DEFAULT '1' NOT NULL, 
	emit_limits TEXT, emit_run_on VARCHAR(64), 
	PRIMARY KEY (id), 
	CONSTRAINT fk_questtemplate_questline_id FOREIGN KEY(questline_id) REFERENCES questline (id), 
	CONSTRAINT fk_questtemplate_category_id FOREIGN KEY(category_id) REFERENCES questcategory (id)
);
CREATE TABLE IF NOT EXISTS "quest" (
	title VARCHAR(200) NOT NULL, 
	description VARCHAR NOT NULL, 
	status VARCHAR NOT NULL, 
	pinned BOOLEAN NOT NULL, 
	sort_order INTEGER NOT NULL, 
	id INTEGER NOT NULL, 
	created_at DATETIME NOT NULL, 
	updated_at DATETIME NOT NULL, 
	completed_at DATETIME, 
	deadline_at DATETIME, 
	duration_seconds INTEGER, 
	template_id INTEGER, 
	period_key VARCHAR(32), 
	significance VARCHAR DEFAULT 'common' NOT NULL, 
	category_id INTEGER, 
	questline_id INTEGER, 
	automated INTEGER DEFAULT '0' NOT NULL, 
	source VARCHAR(48), 
	PRIMARY KEY (id), 
	CONSTRAINT uq_quest_template_period UNIQUE (template_id, period_key), 
	CONSTRAINT fk_quest_questline_id FOREIGN KEY(questline_id) REFERENCES questline (id), 
	CONSTRAINT fk_quest_category_id FOREIGN KEY(category_id) REFERENCES questcategory (id), 
	CONSTRAINT fk_quest_template_id FOREIGN KEY(template_id) REFERENCES questtemplate (id)
);
CREATE TABLE graphdraft_op (
	id INTEGER NOT NULL, 
	kind VARCHAR(32) NOT NULL, 
	payload TEXT NOT NULL, 
	created_at DATETIME NOT NULL, 
	PRIMARY KEY (id)
);
CREATE TABLE graphdraft_state (
	id INTEGER NOT NULL, 
	roots TEXT DEFAULT '[]' NOT NULL, 
	updated_at DATETIME NOT NULL, 
	PRIMARY KEY (id)
);
CREATE TABLE runner (
	id INTEGER NOT NULL, 
	label VARCHAR(64) NOT NULL, 
	reg_hash VARCHAR(64), 
	reg_expires_at DATETIME, 
	token_hash VARCHAR(64), 
	hostname VARCHAR(128), 
	version VARCHAR(32), 
	last_seen_at DATETIME, 
	revoked BOOLEAN DEFAULT 0 NOT NULL, 
	created_at DATETIME NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (label)
);
CREATE TABLE runner_job (
	id INTEGER NOT NULL, 
	runner_label VARCHAR(64) NOT NULL, 
	kind VARCHAR(16) NOT NULL, 
	quest_id INTEGER, 
	step_id INTEGER, 
	template_id INTEGER, 
	title VARCHAR(200) DEFAULT '' NOT NULL, 
	dedupe_key VARCHAR(128), 
	command TEXT NOT NULL, 
	state VARCHAR(12) DEFAULT 'queued' NOT NULL, 
	attempt_no INTEGER DEFAULT '1' NOT NULL, 
	timeout_seconds INTEGER NOT NULL, 
	lease_id VARCHAR(36), 
	lease_until DATETIME, 
	valid_until DATETIME NOT NULL, 
	created_at DATETIME NOT NULL, 
	assigned_at DATETIME, 
	started_at DATETIME, 
	finished_at DATETIME, 
	exit_code INTEGER, 
	timed_out BOOLEAN DEFAULT 0 NOT NULL, 
	stdout TEXT DEFAULT '' NOT NULL, 
	stderr TEXT DEFAULT '' NOT NULL, 
	progress_current INTEGER, 
	progress_total INTEGER, 
	log_tail TEXT DEFAULT '' NOT NULL, 
	reason TEXT DEFAULT '' NOT NULL, 
	cancel BOOLEAN DEFAULT 0 NOT NULL, 
	PRIMARY KEY (id)
);
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
CREATE INDEX ix_note_parent_id ON note (parent_id);
CREATE INDEX ix_note_pinned ON note (pinned);
CREATE INDEX ix_attachment_owner_id ON attachment (owner_id);
CREATE UNIQUE INDEX ix_attachment_webdav_path ON attachment (webdav_path);
CREATE INDEX ix_attachment_owner ON attachment (owner_type, owner_id);
CREATE INDEX ix_attachment_owner_type ON attachment (owner_type);
CREATE INDEX ix_questtemplate_questline_id ON questtemplate (questline_id);
CREATE INDEX ix_questtemplate_category_id ON questtemplate (category_id);
CREATE INDEX ix_quest_template_id ON quest (template_id);
CREATE INDEX ix_quest_category_id ON quest (category_id);
CREATE INDEX ix_quest_period_key ON quest (period_key);
CREATE INDEX ix_quest_questline_id ON quest (questline_id);
CREATE INDEX ix_questchangelog_entity ON questchangelog (entity_type, entity_id);
CREATE INDEX ix_questchangelog_parent ON questchangelog (parent_type, parent_id);
CREATE INDEX ix_runner_job_label_state ON runner_job (runner_label, state);
CREATE UNIQUE INDEX ux_runner_job_open_dedupe ON runner_job (dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('queued','assigned','running');
