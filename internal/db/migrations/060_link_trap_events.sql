-- Мгновенные LINK_UP/LINK_DOWN из SNMP trap (режим + эффекты + флаг на устройстве).
ALTER TABLE snmp_trap_settings
    ADD COLUMN IF NOT EXISTS link_trap_events_mode TEXT NOT NULL DEFAULT 'off';

ALTER TABLE snmp_trap_settings
    ADD COLUMN IF NOT EXISTS link_trap_effects TEXT NOT NULL DEFAULT 'notify';

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS trust_link_traps BOOLEAN NOT NULL DEFAULT false;
