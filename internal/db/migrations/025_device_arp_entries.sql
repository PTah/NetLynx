-- ARP snapshot per device (IP-MIB ipNetToMedia) for port search by IP.

CREATE TABLE IF NOT EXISTS device_arp_entries (
    device_id     BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    ip            TEXT NOT NULL,
    mac           TEXT NOT NULL,
    if_index      INT,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, ip, mac)
);

CREATE INDEX IF NOT EXISTS device_arp_entries_ip_idx ON device_arp_entries (ip);
CREATE INDEX IF NOT EXISTS device_arp_entries_mac_idx ON device_arp_entries (mac);
CREATE INDEX IF NOT EXISTS device_arp_entries_device_idx ON device_arp_entries (device_id);
