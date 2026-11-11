ALTER TABLE entrada
    ADD COLUMN id_mes_ref INTEGER REFERENCES mes_ref (id);

CREATE INDEX idx_entrada_id_mes_ref ON entrada (id_mes_ref);
