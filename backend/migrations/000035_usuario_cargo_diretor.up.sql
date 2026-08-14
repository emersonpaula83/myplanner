-- Cargo "diretor" é novo (feature de cortina de salários): usuários com este
-- cargo poderão destravar a visualização de valores salariais. O CHECK da
-- tabela usuarios ainda não conhece o valor, então o INSERT/UPDATE do handler
-- falharia mesmo com a validação de aplicação já aceitando "diretor".
ALTER TABLE usuarios DROP CONSTRAINT IF EXISTS usuarios_cargo_check;

ALTER TABLE usuarios ADD CONSTRAINT usuarios_cargo_check
  CHECK (cargo IN ('coordenador', 'gerente', 'gerente_projetos', 'diretor'));
