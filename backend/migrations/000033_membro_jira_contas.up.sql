-- Uma pessoa pode ter mais de uma conta no JIRA (Atlassian ID antigo e novo).
-- membros.jira_account_id guarda a conta principal; as contas absorvidas por
-- fusão de membros duplicados ficam aqui. O sync resolve esta tabela antes de
-- inserir, senão a conta absorvida vira uma pessoa nova de novo.
CREATE TABLE membro_jira_contas (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id       UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  fonte_dados_id  UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
  jira_account_id VARCHAR(255) NOT NULL,
  nome_origem     VARCHAR(255),
  criado_em       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (fonte_dados_id, jira_account_id)
);

CREATE INDEX idx_membro_jira_contas_membro ON membro_jira_contas(membro_id);
