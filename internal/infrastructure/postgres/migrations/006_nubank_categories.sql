ALTER TABLE nubank
    ADD COLUMN id_category INTEGER REFERENCES category (id);

UPDATE nubank n
SET id_category = links.id_category
FROM (
    SELECT DISTINCT ON (id_nubank) id_nubank, id_category
    FROM despesa_categoria
    WHERE date_delete IS NULL
    ORDER BY id_nubank, id DESC
) links
WHERE n.id = links.id_nubank;
