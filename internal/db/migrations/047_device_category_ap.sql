-- Категория «точка доступа» для списка «Узлы» и топологии.
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_device_category_check;

ALTER TABLE devices
    ADD CONSTRAINT devices_device_category_check
    CHECK (device_category IN ('switch', 'router', 'ap', 'server', 'computer', 'phone', 'mfu', 'camera', 'other'));
