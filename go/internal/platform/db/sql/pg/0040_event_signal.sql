-- The calendar mark takes the name event. Hook triggers become signals, so the two no longer share a word.
-- GET /api/events stays the live feed; calendar rows are /api/calendar/events.

ALTER TABLE occasion RENAME TO event;
ALTER SEQUENCE occasion_id_seq RENAME TO event_id_seq;
ALTER INDEX ix_occasion_workspace_id RENAME TO ix_event_workspace_id;

ALTER TABLE quest RENAME COLUMN occasion_id TO event_id;
ALTER TABLE quest RENAME COLUMN occasion_edge TO event_edge;
ALTER TABLE quest RENAME COLUMN occasion_dir TO event_dir;
ALTER TABLE quest RENAME COLUMN occasion_offset_seconds TO event_offset_seconds;
ALTER INDEX ix_quest_occasion_id RENAME TO ix_quest_event_id;

ALTER TABLE hook RENAME COLUMN events TO signals;
ALTER TABLE hook RENAME COLUMN events_raw TO signals_raw;

UPDATE script_version SET graph = replace(graph, 'source.event', 'source.signal') WHERE graph LIKE '%source.event%';
UPDATE script_run SET graph_inline = replace(graph_inline, 'source.event', 'source.signal') WHERE graph_inline LIKE '%source.event%';
UPDATE custom_node SET definition = replace(definition, 'source.event', 'source.signal') WHERE definition LIKE '%source.event%';
