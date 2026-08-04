-- Allow SAML users without password
ALTER TABLE usuarios ALTER COLUMN senha_hash DROP NOT NULL;

-- Track auth provider per user
ALTER TABLE usuarios ADD COLUMN auth_provider VARCHAR(20) NOT NULL DEFAULT 'local';

-- Equipe-based access control (replaces projeto-based alçada)
CREATE TABLE IF NOT EXISTS usuario_equipes (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    equipe_id   UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, equipe_id)
);

CREATE INDEX IF NOT EXISTS idx_usuario_equipes_usuario ON usuario_equipes(usuario_id);
