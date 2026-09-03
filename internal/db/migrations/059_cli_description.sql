-- description из show running-config (при открытии карточки / бэкапе).
-- if_descr при sync обновляется из CLI; poller не затирает «железным» Slot:… пока есть cli_description.
ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS cli_description TEXT;
