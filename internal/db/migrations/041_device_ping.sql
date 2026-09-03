-- ICMP reachability для «онлайн» на дашборде (ping OR SNMP).
ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS last_ping_ok boolean,
  ADD COLUMN IF NOT EXISTS last_ping_at timestamptz,
  ADD COLUMN IF NOT EXISTS last_ping_rtt_ms integer;
