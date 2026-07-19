CREATE TABLE usuarios (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome_completo  VARCHAR(255) NOT NULL,
    apelido        VARCHAR(50) NOT NULL UNIQUE,
    email          VARCHAR(255) NOT NULL UNIQUE,
    senha_hash     VARCHAR(255) NOT NULL,
    cargo          VARCHAR(50) NOT NULL CHECK (cargo IN ('coordenador', 'gerente', 'gerente_projetos')),
    ativo          BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_usuarios_email ON usuarios(email);

CREATE TABLE usuario_projetos (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    projeto_id  UUID NOT NULL REFERENCES projetos(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, projeto_id)
);

CREATE INDEX idx_usuario_projetos_usuario ON usuario_projetos(usuario_id);

-- Seed admin user
-- Password: Totvs@123 (bcrypt cost 12)
INSERT INTO usuarios (nome_completo, apelido, email, senha_hash, cargo)
VALUES (
    'Administrador',
    'admin',
    'admin@myplanner.local',
    '$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK',
    'coordenador'
);

-- Grant admin access to all existing projects
INSERT INTO usuario_projetos (usuario_id, projeto_id)
SELECT u.id, p.id
FROM usuarios u, projetos p
WHERE u.apelido = 'admin';
