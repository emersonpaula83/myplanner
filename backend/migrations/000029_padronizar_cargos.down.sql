ALTER TABLE membros DROP CONSTRAINT IF EXISTS membros_cargo_check;

ALTER TABLE membros ADD CONSTRAINT membros_cargo_check
  CHECK (cargo IN (
    'coordenador_desenvolvimento',
    'po_produto',
    'gerente_tecnologia',
    'gerente_executivo',
    'scrum_master',
    'agile_master',
    'desenvolvedor'
  ));
