DROP TABLE IF EXISTS membro_banco_horas;
DROP TABLE IF EXISTS membro_salarios;
ALTER TABLE membros DROP COLUMN IF EXISTS banco_horas;
ALTER TABLE membros DROP COLUMN IF EXISTS data_admissao;
ALTER TABLE membros DROP COLUMN IF EXISTS salario;
