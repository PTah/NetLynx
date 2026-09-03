-- Категория «телефон» для списка «Узлы».
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_device_category_check;

ALTER TABLE devices
    ADD CONSTRAINT devices_device_category_check
    CHECK (device_category IN ('switch', 'router', 'server', 'computer', 'phone', 'mfu', 'camera', 'other'));
