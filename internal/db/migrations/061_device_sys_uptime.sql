-- sysUpTime (TimeTicks) при последнем успешном SNMP-опросе
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_sys_uptime_cs BIGINT;
