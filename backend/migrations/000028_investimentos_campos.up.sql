-- Add financial fields to membros
ALTER TABLE membros ADD COLUMN salario DECIMAL(12,2);
ALTER TABLE membros ADD COLUMN data_admissao DATE;
ALTER TABLE membros ADD COLUMN banco_horas DECIMAL(8,2) DEFAULT 0;

-- Salary history
CREATE TABLE membro_salarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(12,2) NOT NULL,
    data_vigencia DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_membro_salarios_membro ON membro_salarios(membro_id, data_vigencia);

-- Hours bank history
CREATE TABLE membro_banco_horas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(8,2) NOT NULL,
    data_registro TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_membro_banco_horas_membro ON membro_banco_horas(membro_id, data_registro);
