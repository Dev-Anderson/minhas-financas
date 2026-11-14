ALTER TABLE expense
    ADD COLUMN id_mes_ref INTEGER REFERENCES mes_ref (id);

CREATE INDEX idx_expense_id_mes_ref ON expense (id_mes_ref);
