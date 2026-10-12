-- Настройки расследования MAC: WiFi-клиенты на AP (по ARP-подсети).
CREATE TABLE IF NOT EXISTS mac_investigation_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    track_wifi_clients BOOLEAN NOT NULL DEFAULT false,
    wifi_client_ip_prefix TEXT NOT NULL DEFAULT '192.168.120.0/24',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO mac_investigation_settings (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;
