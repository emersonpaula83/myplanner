# Cargos de Membros — Design Spec

## Objetivo

Associar um cargo (função) a cada membro do sistema. Cargo é global (não varia por equipe). Lista de cargos é fixa (enum). Quando cargo = P.O. Produto, membro deve ter 1+ produtos (componentes JIRA) associados.

## Requisitos

1. Cada membro pode ter 0 ou 1 cargo
2. 7 cargos fixos (hardcoded):
   - `coordenador_desenvolvimento` → Coordenador de Desenvolvimento
   - `po_produto` → P.O. Produto
   - `gerente_tecnologia` → Gerente de Tecnologia
   - `gerente_executivo` → Gerente Executivo
   - `scrum_master` → Scrum Master
   - `agile_master` → Agile Master
   - `desenvolvedor` → Desenvolvedor
3. P.O. Produto obriga seleção de 1+ produtos (tabela `produtos` já existente, populada via sync JIRA)
4. Atribuição de cargo na página detalhe do membro
5. Badge/tag de cargo visível na listagem de membros da equipe

## Banco de Dados

### Migration 000014

```sql
ALTER TABLE membros ADD COLUMN cargo VARCHAR(50)
  CHECK (cargo IN (
    'coordenador_desenvolvimento',
    'po_produto',
    'gerente_tecnologia',
    'gerente_executivo',
    'scrum_master',
    'agile_master',
    'desenvolvedor'
  ));

CREATE TABLE membro_produtos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  produto_id UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(membro_id, produto_id)
);

CREATE INDEX idx_membro_produtos_membro ON membro_produtos(membro_id);
CREATE INDEX idx_membro_produtos_produto ON membro_produtos(produto_id);
```

### Down migration

```sql
DROP TABLE IF EXISTS membro_produtos;
ALTER TABLE membros DROP COLUMN IF EXISTS cargo;
```

## Backend

### Domain (`domain/models.go`)

- Adicionar `Cargo *string` no struct `Membro`
- Constantes Go para enum de cargos:
  ```go
  const (
      CargoCoordenadorDesenvolvimento = "coordenador_desenvolvimento"
      CargoPOProduto                  = "po_produto"
      CargoGerenteTecnologia          = "gerente_tecnologia"
      CargoGerenteExecutivo           = "gerente_executivo"
      CargoScrumMaster                = "scrum_master"
      CargoAgileMaster                = "agile_master"
      CargoDesenvolvedor              = "desenvolvedor"
  )

  var CargosValidos = []string{
      CargoCoordenadorDesenvolvimento,
      CargoPOProduto,
      CargoGerenteTecnologia,
      CargoGerenteExecutivo,
      CargoScrumMaster,
      CargoAgileMaster,
      CargoDesenvolvedor,
  }

  var CargoLabels = map[string]string{
      CargoCoordenadorDesenvolvimento: "Coordenador de Desenvolvimento",
      CargoPOProduto:                  "P.O. Produto",
      CargoGerenteTecnologia:          "Gerente de Tecnologia",
      CargoGerenteExecutivo:           "Gerente Executivo",
      CargoScrumMaster:                "Scrum Master",
      CargoAgileMaster:                "Agile Master",
      CargoDesenvolvedor:              "Desenvolvedor",
  }
  ```
- Função `IsCargoValido(cargo string) bool`

### Repository (`repository/equipe.go`)

- `GetMembrosEquipe` — incluir `m.cargo` no SELECT
- `UpdateMembroCargo(ctx, membroID uuid.UUID, cargo *string) error` — UPDATE `membros SET cargo = $1 WHERE id = $2`
- `ListProdutos(ctx) ([]domain.Produto, error)` — SELECT * FROM produtos WHERE ativo = true ORDER BY nome
- `GetMembroProdutos(ctx, membroID uuid.UUID) ([]domain.Produto, error)` — SELECT via JOIN membro_produtos
- `SetMembroProdutos(ctx, membroID uuid.UUID, produtoIDs []uuid.UUID) error` — DELETE all para membro + INSERT novos (transação)

### Handler (`handler/equipe.go`)

Novos endpoints:

| Método | Rota | Body | Descrição |
|--------|------|------|-----------|
| PUT | `/membros/{id}/cargo` | `{"cargo": "po_produto"}` ou `{"cargo": null}` | Atualiza cargo do membro |
| GET | `/membros/{id}/produtos` | — | Lista produtos do membro |
| PUT | `/membros/{id}/produtos` | `{"produto_ids": ["uuid1", ...]}` | Substitui produtos do membro |
| GET | `/produtos` | — | Lista todos produtos ativos |

Validação no handler: `PUT /membros/{id}/cargo` valida que cargo está na lista de `CargosValidos` (ou null para remover).

### Interface `EquipeStore`

Adicionar métodos:
- `UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error`
- `ListProdutos(ctx context.Context) ([]domain.Produto, error)`
- `GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error)`
- `SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error`

## Frontend (`frontend/index.html`)

### Página detalhe do membro (`page-membro-detail`)

Nova seção "Cargo" entre dados pessoais e skills:

1. **Dropdown cargo:**
   - `<select>` com opção vazia "Sem cargo" + 7 opções
   - Ao mudar, `PUT /membros/{id}/cargo`
   - Valor vazio envia `{"cargo": null}`

2. **Seção Produtos (condicional — só quando cargo = `po_produto`):**
   - Lista de produtos associados como chips com botão X para remover
   - Input autocomplete para buscar e adicionar produtos (GET `/produtos`, filtro client-side)
   - Mínimo 1 produto obrigatório — botão X desabilitado quando só resta 1
   - Ao mudar cargo para outro valor, seção Produtos some (associações mantidas no banco)

### Listagem membros da equipe (`renderEquipeResumo`)

Badge/tag colorida ao lado do nome do membro:

| Cargo | Cor |
|-------|-----|
| Coordenador de Desenvolvimento | Azul |
| P.O. Produto | Roxo |
| Gerente de Tecnologia | Azul |
| Gerente Executivo | Azul |
| Scrum Master | Verde |
| Agile Master | Verde |
| Desenvolvedor | Cinza |
| Sem cargo | Sem badge |

### Validação P.O. no frontend

- Quando cargo muda para `po_produto`: seção produtos aparece
- Se nenhum produto associado: mostrar alerta inline "Selecione ao menos 1 produto"
- Impedir troca de cargo até ter 1+ produto (validação client-side)
- Backend aceita cargo sem produtos (flexibilidade) mas UI impõe regra

## Fora de escopo

- CRUD de cargos (lista fixa)
- Cargo por equipe (é global)
- Múltiplos cargos por membro
- Validação backend P.O. sem produtos (apenas client-side)
- Impacto no cálculo de capacidade por cargo
