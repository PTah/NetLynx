-- PoE: факт выдачи питания (SNMP POWER-ETHERNET-MIB). NULL = ещё не определено / MIB недоступен.
ALTER TABLE device_interfaces
    ADD COLUMN IF NOT EXISTS poe_active BOOLEAN;
