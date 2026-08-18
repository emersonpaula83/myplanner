CREATE TABLE usuario_projeto_favoritos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(usuario_id, fonte_dados_id, project_key)
);
CREATE INDEX idx_upf_usuario_fonte ON usuario_projeto_favoritos(usuario_id, fonte_dados_id);
