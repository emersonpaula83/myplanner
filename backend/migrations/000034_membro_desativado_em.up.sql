-- Marca desativação manual de membro. Sem isso o sync não distingue "conta que
-- ninguém desativou" de "registro duplicado que um operador tirou do ar": o
-- upsert de membros força ativo = true toda vez que a conta reaparece no JIRA,
-- e o duplicado voltaria para as listas sozinho.
ALTER TABLE membros ADD COLUMN desativado_em TIMESTAMPTZ;
