-- Printer-MIB snapshot on devices (page count + toner supplies JSON)
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_page_count BIGINT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_toners JSONB;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_printer_at TIMESTAMPTZ;
