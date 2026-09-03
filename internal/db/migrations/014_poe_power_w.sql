-- PoE: потребляемая/отдаваемая мощность на порту (ватты), если доступно по SNMP.
ALTER TABLE device_interfaces
    ADD COLUMN IF NOT EXISTS poe_power_w REAL;
