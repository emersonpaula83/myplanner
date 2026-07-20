CREATE TABLE feriados (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    data        DATE NOT NULL,
    nome        VARCHAR(255) NOT NULL,
    tipo        VARCHAR(50) NOT NULL DEFAULT 'nacional',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(data)
);

CREATE INDEX idx_feriados_data ON feriados(data);

-- Feriados nacionais brasileiros 2025-2027
INSERT INTO feriados (data, nome, tipo) VALUES
    -- 2025
    ('2025-01-01', 'Confraternização Universal', 'nacional'),
    ('2025-03-03', 'Carnaval', 'nacional'),
    ('2025-03-04', 'Carnaval', 'nacional'),
    ('2025-04-18', 'Sexta-feira Santa', 'nacional'),
    ('2025-04-21', 'Tiradentes', 'nacional'),
    ('2025-05-01', 'Dia do Trabalho', 'nacional'),
    ('2025-06-19', 'Corpus Christi', 'nacional'),
    ('2025-09-07', 'Independência do Brasil', 'nacional'),
    ('2025-10-12', 'Nossa Senhora Aparecida', 'nacional'),
    ('2025-11-02', 'Finados', 'nacional'),
    ('2025-11-15', 'Proclamação da República', 'nacional'),
    ('2025-11-20', 'Dia da Consciência Negra', 'nacional'),
    ('2025-12-25', 'Natal', 'nacional'),
    -- 2026
    ('2026-01-01', 'Confraternização Universal', 'nacional'),
    ('2026-02-16', 'Carnaval', 'nacional'),
    ('2026-02-17', 'Carnaval', 'nacional'),
    ('2026-04-03', 'Sexta-feira Santa', 'nacional'),
    ('2026-04-21', 'Tiradentes', 'nacional'),
    ('2026-05-01', 'Dia do Trabalho', 'nacional'),
    ('2026-06-04', 'Corpus Christi', 'nacional'),
    ('2026-09-07', 'Independência do Brasil', 'nacional'),
    ('2026-10-12', 'Nossa Senhora Aparecida', 'nacional'),
    ('2026-11-02', 'Finados', 'nacional'),
    ('2026-11-15', 'Proclamação da República', 'nacional'),
    ('2026-11-20', 'Dia da Consciência Negra', 'nacional'),
    ('2026-12-25', 'Natal', 'nacional'),
    -- 2027
    ('2027-01-01', 'Confraternização Universal', 'nacional'),
    ('2027-02-08', 'Carnaval', 'nacional'),
    ('2027-02-09', 'Carnaval', 'nacional'),
    ('2027-03-26', 'Sexta-feira Santa', 'nacional'),
    ('2027-04-21', 'Tiradentes', 'nacional'),
    ('2027-05-01', 'Dia do Trabalho', 'nacional'),
    ('2027-05-27', 'Corpus Christi', 'nacional'),
    ('2027-09-07', 'Independência do Brasil', 'nacional'),
    ('2027-10-12', 'Nossa Senhora Aparecida', 'nacional'),
    ('2027-11-02', 'Finados', 'nacional'),
    ('2027-11-15', 'Proclamação da República', 'nacional'),
    ('2027-11-20', 'Dia da Consciência Negra', 'nacional'),
    ('2027-12-25', 'Natal', 'nacional')
ON CONFLICT (data) DO NOTHING;
