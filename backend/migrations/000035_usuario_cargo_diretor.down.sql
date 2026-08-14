-- Reverte para o conjunto de cargos anterior à feature de cortina de
-- salários. Downgrade só é seguro se nenhuma linha ficou com cargo='diretor';
-- quem rodar down é responsável por migrar esses usuários antes.
ALTER TABLE usuarios DROP CONSTRAINT IF EXISTS usuarios_cargo_check;

ALTER TABLE usuarios ADD CONSTRAINT usuarios_cargo_check
  CHECK (cargo IN ('coordenador', 'gerente', 'gerente_projetos'));
