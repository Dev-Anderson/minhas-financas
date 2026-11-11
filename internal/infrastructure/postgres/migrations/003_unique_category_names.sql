CREATE UNIQUE INDEX idx_category_unique_active_name
    ON category (LOWER(BTRIM(name)))
    WHERE date_delete IS NULL;
