CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Fonte de Dados (JIRA connections)
CREATE TABLE fonte_dados (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome            VARCHAR(255) NOT NULL,
    tipo            VARCHAR(50) NOT NULL DEFAULT 'jira',
    base_url        VARCHAR(512) NOT NULL,
    auth_type       VARCHAR(20) NOT NULL,
    api_token       TEXT,
    user_email      VARCHAR(255),
    oauth2_client_id     VARCHAR(255),
    oauth2_client_secret TEXT,
    oauth2_access_token  TEXT,
    oauth2_refresh_token TEXT,
    oauth2_token_expiry  TIMESTAMPTZ,
    custom_field_map JSONB NOT NULL DEFAULT '{}',
    ativo           BOOLEAN NOT NULL DEFAULT true,
    ultimo_sync     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Membros (JIRA users)
CREATE TABLE membros (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    jira_account_id VARCHAR(255) NOT NULL,
    nome            VARCHAR(255) NOT NULL,
    email           VARCHAR(255),
    avatar_url      VARCHAR(512),
    team            VARCHAR(255),
    ativo           BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, jira_account_id)
);

-- Projetos (JIRA spaces/projects)
CREATE TABLE projetos (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    jira_id         VARCHAR(50) NOT NULL,
    chave           VARCHAR(50) NOT NULL,
    nome            VARCHAR(255) NOT NULL,
    descricao       TEXT,
    lead_id         UUID REFERENCES membros(id),
    categoria       VARCHAR(255),
    ativo           BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, jira_id)
);

-- Sprints
CREATE TABLE sprints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    projeto_id      UUID REFERENCES projetos(id) ON DELETE SET NULL,
    jira_id         INTEGER NOT NULL,
    nome            VARCHAR(255) NOT NULL,
    estado          VARCHAR(50),
    data_inicio     TIMESTAMPTZ,
    data_fim        TIMESTAMPTZ,
    data_conclusao  TIMESTAMPTZ,
    board_id        INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, jira_id)
);

-- Produtos (JIRA Components)
CREATE TABLE produtos (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    jira_id         VARCHAR(50) NOT NULL,
    nome            VARCHAR(255) NOT NULL,
    descricao       TEXT,
    projeto_id      UUID REFERENCES projetos(id) ON DELETE CASCADE,
    ativo           BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, jira_id)
);

CREATE INDEX idx_produtos_fonte ON produtos(fonte_dados_id);
CREATE INDEX idx_produtos_projeto ON produtos(projeto_id);
CREATE INDEX idx_produtos_nome ON produtos(nome);

-- Tarefas (JIRA issues)
CREATE TABLE tarefas (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id      UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    projeto_id          UUID NOT NULL REFERENCES projetos(id) ON DELETE CASCADE,
    jira_id             VARCHAR(50) NOT NULL,
    numero_ticket       VARCHAR(50) NOT NULL,
    resumo              TEXT NOT NULL,
    tipo                VARCHAR(100) NOT NULL,
    status              VARCHAR(100) NOT NULL,
    prioridade          VARCHAR(100),
    estimativa_pontos   DECIMAL(10,2),
    estimativa_tempo    INTEGER,
    tempo_gasto         INTEGER,
    responsavel_id      UUID REFERENCES membros(id) ON DELETE SET NULL,
    relator_id          UUID REFERENCES membros(id) ON DELETE SET NULL,
    team                VARCHAR(255),
    sprint_id           UUID REFERENCES sprints(id) ON DELETE SET NULL,
    data_criacao        TIMESTAMPTZ NOT NULL,
    data_limite         DATE,
    data_resolvido      TIMESTAMPTZ,
    data_atualizado     TIMESTAMPTZ,
    tipo_demanda        VARCHAR(255),
    data_componente     DATE,
    status_categoria    VARCHAR(50),
    campos_extras       JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, jira_id)
);

CREATE INDEX idx_tarefas_capacity ON tarefas(responsavel_id, status_categoria, estimativa_pontos);
CREATE INDEX idx_tarefas_throughput ON tarefas(projeto_id, data_resolvido, status_categoria);
CREATE INDEX idx_tarefas_team_status ON tarefas(team, status, data_criacao);
CREATE INDEX idx_tarefas_data_resolvido ON tarefas(data_resolvido);
CREATE INDEX idx_tarefas_data_limite ON tarefas(data_limite);

-- Tarefa <-> Produto (N:N)
CREATE TABLE tarefa_produtos (
    tarefa_id       UUID NOT NULL REFERENCES tarefas(id) ON DELETE CASCADE,
    produto_id      UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
    PRIMARY KEY (tarefa_id, produto_id)
);

CREATE INDEX idx_tarefa_produtos_produto ON tarefa_produtos(produto_id);
CREATE INDEX idx_tarefa_produtos_tarefa ON tarefa_produtos(tarefa_id);

-- Limites de Alerta (configurable thresholds)
CREATE TABLE limites_alerta (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome            VARCHAR(255) NOT NULL,
    descricao       TEXT,
    escopo          VARCHAR(20) NOT NULL DEFAULT 'global',
    referencia_id   UUID,
    metrica         VARCHAR(50) NOT NULL DEFAULT 'percentual_atraso',
    limite_verde    DECIMAL(10,2) NOT NULL DEFAULT 10,
    limite_amarelo  DECIMAL(10,2) NOT NULL DEFAULT 20,
    limite_laranja  DECIMAL(10,2) NOT NULL DEFAULT 35,
    padrao          BOOLEAN NOT NULL DEFAULT false,
    ativo           BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO limites_alerta (nome, descricao, escopo, metrica, limite_verde, limite_amarelo, limite_laranja, padrao)
VALUES ('Padrão', 'Limites padrão para atraso de tarefas', 'global', 'percentual_atraso', 10, 20, 35, true);

-- Disponibilidade (absences)
CREATE TABLE disponibilidade (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id       UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    tipo            VARCHAR(50) NOT NULL,
    data_inicio     DATE NOT NULL,
    data_fim        DATE NOT NULL,
    descricao       TEXT,
    criado_por      VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_disponibilidade_membro ON disponibilidade(membro_id);
CREATE INDEX idx_disponibilidade_datas ON disponibilidade(data_inicio, data_fim);

-- Sync Logs
CREATE TABLE sync_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    tipo            VARCHAR(20) NOT NULL,
    status          VARCHAR(20) NOT NULL,
    iniciado_em     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finalizado_em   TIMESTAMPTZ,
    total_projetos  INTEGER DEFAULT 0,
    total_tarefas   INTEGER DEFAULT 0,
    total_membros   INTEGER DEFAULT 0,
    total_sprints   INTEGER DEFAULT 0,
    erros           JSONB DEFAULT '[]',
    mensagem        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
