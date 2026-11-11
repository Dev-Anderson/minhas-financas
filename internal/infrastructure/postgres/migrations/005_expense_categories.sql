ALTER TABLE expense
    ADD COLUMN id_category INTEGER REFERENCES category (id);
