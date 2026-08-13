-- Backfill membro_salarios for members with salary but no history record.
-- Uses ultimo_aumento as data_vigencia (all 33 members have it set).
INSERT INTO membro_salarios (membro_id, valor, data_vigencia)
SELECT m.id, m.salario, COALESCE(m.ultimo_aumento, m.data_admissao, CURRENT_DATE)
FROM membros m
WHERE m.salario IS NOT NULL
  AND m.id NOT IN (SELECT DISTINCT membro_id FROM membro_salarios);
