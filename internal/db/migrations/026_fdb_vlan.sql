-- VLAN per MAC в FDB (Q-BRIDGE), для разворота клиентов на порту.

ALTER TABLE device_fdb_entries ADD COLUMN IF NOT EXISTS vlan_id INT;
