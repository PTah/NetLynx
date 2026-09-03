export type Device = {
  id: number;
  name: string;
  host: string;
  location?: string | null;
  /** switch | router | ap | server | computer | phone | mfu | camera | other */
  device_category?: string;
  uisp_device_id?: string | null;
  /** Состояние связи с UISP (overview.status): active, disconnected, … */
  uisp_overview_status?: string | null;
  snmp_version: string;
  community?: string | null;
  has_community?: boolean;
  ssh_user?: string | null;
  ssh_port?: number | null;
  ssh_vendor?: string | null;
  has_ssh_password?: boolean;
  has_ssh_enable_password?: boolean;
  v3_user?: string | null;
  v3_auth_protocol?: string | null;
  v3_priv_protocol?: string | null;
  v3_engine_id?: string | null;
  poll_interval_seconds: number;
  last_snmp_ok?: boolean | null;
  last_snmp_error?: string | null;
  /** ICMP ping с сервера NetLynx */
  last_ping_ok?: boolean | null;
  last_ping_at?: string | null;
  last_ping_rtt_ms?: number | null;
  /** null = авто; true/false = ручная отметка онлайн/оффлайн */
  online_override?: boolean | null;
  /** При mode=per_device — мгновенные LINK_* из SNMP trap */
  trust_link_traps?: boolean;
  /** ISO: когда узел стал оффлайн (если сейчас оффлайн) */
  offline_since?: string | null;
  sys_name?: string | null;
  /** Chassis MAC (LLDP loc / bridge), для резолва соседей */
  chassis_mac?: string | null;
  last_poll_at?: string | null;
  cpu_profile?: string | null;
  last_cpu_pct?: number | null;
  last_cpu_at?: string | null;
  /** sysUpTime (SNMP TimeTicks, сотые доли секунды) при last_poll_at */
  last_sys_uptime_cs?: number | null;
  fdb_monitoring_status?: string | null;
  util_high_pct?: number | null;
  util_ok_pct?: number | null;
  fdb_poll_interval_seconds?: number | null;
};

export type EventRow = {
  id: number;
  device_id: number;
  if_index?: number | null;
  event_type: string;
  severity: string;
  payload?: Record<string, unknown>;
  created_at: string;
};
