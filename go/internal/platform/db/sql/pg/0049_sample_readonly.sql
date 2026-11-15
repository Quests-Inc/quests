-- Protect samples at the storage boundary, including script/graph writes and
-- concurrent flag changes. Unmarking is a separate operation from editing.
CREATE FUNCTION protect_sample_note() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.is_sample = 1 THEN
    IF TG_OP = 'DELETE' OR
       ROW(NEW.title, NEW.description, NEW.parent_id, NEW.pinned, NEW.is_readme, NEW.is_private, NEW.is_category, NEW.sort_order, NEW.color, NEW.icon, NEW.custom_icon, NEW.deleted_at, NEW.check_command, NEW.check_interval_seconds, NEW.run_on) IS DISTINCT FROM
       ROW(OLD.title, OLD.description, OLD.parent_id, OLD.pinned, OLD.is_readme, OLD.is_private, OLD.is_category, OLD.sort_order, OLD.color, OLD.icon, OLD.custom_icon, OLD.deleted_at, OLD.check_command, OLD.check_interval_seconds, OLD.run_on) THEN
      RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = format('Заметка «%s» помечена как образец. Редактирование запрещено', OLD.title);
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER sample_note_readonly BEFORE UPDATE OR DELETE ON note
FOR EACH ROW EXECUTE FUNCTION protect_sample_note();

CREATE FUNCTION protect_sample_environment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  owner_id bigint;
  sample_title text;
  sample_flag integer;
BEGIN
  -- Check both sides of a move, as well as inserts, updates and deletions.
  FOR owner_id IN SELECT DISTINCT id FROM unnest(ARRAY[
    CASE WHEN TG_OP <> 'INSERT' THEN OLD.note_id END,
    CASE WHEN TG_OP <> 'DELETE' THEN NEW.note_id END
  ]) AS owners(id) WHERE id IS NOT NULL LOOP
    SELECT title, is_sample INTO sample_title, sample_flag FROM note WHERE id = owner_id FOR SHARE;
    IF FOUND AND sample_flag = 1 THEN
      RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = format('Заметка «%s» помечена как образец. Редактирование запрещено', sample_title);
    END IF;
  END LOOP;
  IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER sample_values_readonly BEFORE INSERT OR UPDATE OR DELETE ON named_value
FOR EACH ROW EXECUTE FUNCTION protect_sample_environment();
CREATE TRIGGER sample_secrets_readonly BEFORE INSERT OR UPDATE OR DELETE ON secret
FOR EACH ROW EXECUTE FUNCTION protect_sample_environment();
