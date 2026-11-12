-- Custom image for a calendar event, same idea as questline/note icons.
ALTER TABLE event ADD COLUMN IF NOT EXISTS custom_icon TEXT;
