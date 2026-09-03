-- Журнал последнего запуска бэкапа (шаги для UI).

ALTER TABLE backup_settings
    ADD COLUMN IF NOT EXISTS last_log TEXT;
