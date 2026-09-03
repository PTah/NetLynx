-- Фильтр типов trap для тестового журнала (CSV trap_label; пусто = все).
ALTER TABLE snmp_trap_settings
    ADD COLUMN IF NOT EXISTS trap_include_labels TEXT;
