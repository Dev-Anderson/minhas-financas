DROP INDEX IF EXISTS idx_mes_ref_name;

CREATE UNIQUE INDEX idx_mes_ref_active_name
    ON mes_ref (LOWER(BTRIM(name)))
    WHERE date_delete IS NULL;
