ALTER TABLE upstream_bill_reconciliations
    ADD COLUMN IF NOT EXISTS source_file_name VARCHAR(255);
