-- Drop old CHECK constraint
ALTER TABLE membros DROP CONSTRAINT IF EXISTS membros_cargo_check;

-- Migrate desenvolvedor -> analista_iii
UPDATE membros SET cargo = 'analista_iii' WHERE cargo = 'desenvolvedor';

-- Cargos removidos -> NULL
UPDATE membros SET cargo = NULL WHERE cargo IN (
  'po_produto', 'gerente_tecnologia', 'gerente_executivo',
  'scrum_master', 'agile_master'
);

-- Recreate CHECK constraint with new values
ALTER TABLE membros ADD CONSTRAINT membros_cargo_check
  CHECK (cargo IN (
    'analista_i',
    'analista_ii',
    'analista_iii',
    'especialista_i',
    'especialista_ii',
    'master',
    'coordenador_desenvolvimento',
    'lider_tecnico'
  ));
