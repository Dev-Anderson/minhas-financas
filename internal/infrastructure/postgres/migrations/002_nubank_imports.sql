CREATE UNIQUE INDEX idx_mes_ref_name ON mes_ref (name);

CREATE TABLE nubank_imports (
    file_hash CHAR(64) PRIMARY KEY,
    row_count INTEGER NOT NULL CHECK (row_count > 0),
    imported_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
