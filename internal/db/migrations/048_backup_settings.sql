-- Глобальные настройки резервных копий (singleton id=1).

CREATE TABLE IF NOT EXISTS backup_settings (
    id                      SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    schedule_enabled        BOOLEAN NOT NULL DEFAULT false,
    schedule_hour           INT NOT NULL DEFAULT 2 CHECK (schedule_hour >= 0 AND schedule_hour <= 23),
    schedule_minute         INT NOT NULL DEFAULT 0 CHECK (schedule_minute >= 0 AND schedule_minute <= 59),
    local_enabled           BOOLEAN NOT NULL DEFAULT true,
    local_dir               TEXT NOT NULL DEFAULT '/var/backups/netlynx',
    local_retain_days       INT NOT NULL DEFAULT 3 CHECK (local_retain_days >= 1 AND local_retain_days <= 365),
    email_enabled           BOOLEAN NOT NULL DEFAULT false,
    email_to                TEXT,
    share_enabled           BOOLEAN NOT NULL DEFAULT false,
    share_kind              TEXT NOT NULL DEFAULT 'smb' CHECK (share_kind IN ('smb', 'nfs')),
    share_url               TEXT,
    share_username          TEXT,
    share_password          TEXT,
    share_domain            TEXT,
    share_retain_days       INT NOT NULL DEFAULT 3 CHECK (share_retain_days >= 1 AND share_retain_days <= 365),
    switch_cfg_enabled      BOOLEAN NOT NULL DEFAULT false,
    ssh_user                TEXT,
    ssh_password            TEXT,
    ssh_port                INT NOT NULL DEFAULT 22 CHECK (ssh_port > 0 AND ssh_port <= 65535),
    ssh_enable_password     TEXT,
    ssh_timeout_seconds     INT NOT NULL DEFAULT 30 CHECK (ssh_timeout_seconds >= 5 AND ssh_timeout_seconds <= 300),
    last_run_at             TIMESTAMPTZ,
    last_status             TEXT,
    last_error              TEXT,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO backup_settings (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;
