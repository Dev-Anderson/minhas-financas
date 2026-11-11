CREATE TABLE mes_ref (
    id SERIAL PRIMARY KEY,
    name VARCHAR,
    date_create TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_delete TIMESTAMP
);

CREATE TABLE expense (
    id SERIAL PRIMARY KEY,
    name VARCHAR,
    valor NUMERIC,
    date_create TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_delete TIMESTAMP
);

CREATE TABLE category (
    id SERIAL PRIMARY KEY,
    name VARCHAR,
    date_create TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_delete TIMESTAMP
);

CREATE TABLE nubank (
    id SERIAL PRIMARY KEY,
    id_mes_ref INTEGER NOT NULL REFERENCES mes_ref (id),
    date TIMESTAMP,
    title VARCHAR,
    parcela VARCHAR,
    amount NUMERIC
);

CREATE TABLE entrada (
    id SERIAL PRIMARY KEY,
    name VARCHAR,
    valor NUMERIC,
    date_create TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_delete TIMESTAMP
);

CREATE TABLE despesa_categoria (
    id SERIAL PRIMARY KEY,
    id_nubank INTEGER NOT NULL REFERENCES nubank (id),
    id_expense INTEGER NOT NULL REFERENCES expense (id),
    id_category INTEGER NOT NULL REFERENCES category (id),
    date_create TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    date_delete TIMESTAMP
);

CREATE INDEX idx_nubank_id_mes_ref ON nubank (id_mes_ref);
CREATE INDEX idx_despesa_categoria_id_nubank ON despesa_categoria (id_nubank);
CREATE INDEX idx_despesa_categoria_id_expense ON despesa_categoria (id_expense);
CREATE INDEX idx_despesa_categoria_id_category ON despesa_categoria (id_category);
