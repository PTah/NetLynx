-- Ручная переопределение «онлайн» для узлов без ping/SNMP (файрвол и т.п.).
-- NULL = автоматически; true = считать онлайн; false = считать оффлайн.
ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS online_override BOOLEAN NULL;
