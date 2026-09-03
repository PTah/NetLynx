export type TopologyNode = {
  id: number;
  name: string;
  host: string;
  sys_name?: string | null;
  sys_descr?: string | null;
  location?: string | null;
  last_snmp_ok?: boolean | null;
  last_ping_ok?: boolean | null;
  online_override?: boolean | null;
  uisp_device_id?: string | null;
  uisp_overview_status?: string | null;
  virtual?: boolean;
  kind?: string;
  link_count?: number;
  discovered_id?: number | null;
};

export type TopologyEdge = {
  local_device_id: number;
  local_if_index: number;
  local_if_name?: string | null;
  local_if_speed?: number | null;
  poe_active?: boolean | null;
  poe_power_w?: number | null;
  vlan_id?: number | null;
  remote_device_id?: number | null;
  remote_sys_name?: string | null;
  remote_port_id?: string | null;
  remote_if_name?: string | null;
  remote_chassis_id?: string | null;
  remote_mgmt_addr?: string | null;
  protocol: string;
  protocols?: string[];
  rem_index: number;
  stale: boolean;
  last_seen_at?: string | null;
  unresolved_label?: string;
  manual_link_id?: number | null;
  manual_note?: string | null;
};

export type ManualTopologyLink = {
  id: number;
  a_device_id: number;
  a_if_index: number;
  b_device_id: number;
  b_if_index: number;
  note?: string | null;
  status: string;
  superseded_at?: string | null;
  superseded_by?: string | null;
  created_by?: string | null;
  created_at: string;
  updated_at: string;
  a_device_name?: string | null;
  b_device_name?: string | null;
};

export type TopologyGraph = {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
};

export type PortSearchHit = {
  device_id: number;
  device_name: string;
  device_host: string;
  if_index: number;
  if_name?: string | null;
  mac?: string | null;
  ip?: string | null;
  match_type: string;
  note?: string | null;
};
