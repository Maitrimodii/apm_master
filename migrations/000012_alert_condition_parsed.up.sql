-- Parsed form of alerts.condition, written when an alert is saved so the
-- evaluator and the AI service never re-parse the text. NULL for rows saved
-- before this column existed; they are parsed on read until next update.
ALTER TABLE alerts ADD COLUMN condition_parsed JSONB;
