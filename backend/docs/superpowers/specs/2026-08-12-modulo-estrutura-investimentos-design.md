# Módulo Estrutura — Investimentos

> **Meta:** Dashboard financeiro por equipe com visão de investimentos em pessoas, banco de horas e atuação por produto.

**Escopo:** Backend Go (SP1) + Frontend vanilla JS (SP2). Dois sub-projetos sequenciais.

**Abordagem:** Módulo monolítico — schema, APIs e frontend entregues como unidade coesa.

---

## Global Constraints

- Frontend: vanilla JS/HTML/CSS — sem frameworks, sem dependências externas
- Gráficos: SVG custom (consistente com `drawPieChart` existente)
- Acesso: mesmo controle de equipes — coordenador vê 1:N equipes
- Dados financeiros editados no detalhe do membro (módulo Equipes existente)
- Histórico salarial e banco de horas registrados automaticamente ao alterar valor
- Backend: Go, pgxpool, padrão handler/service/repository existente
- Sem alteração no módulo de alocação — dados consumidos read-only

---

## 1. Reestruturação de Menu

Menu lateral atual tem "Equipes" no nível raiz. Muda para:

```
Estrutura (novo grupo, ícone organograma)
  ├── Equipes (existente, move pra dentro)
  └── Investimentos (novo)
```

Navegação: `data-page="investimentos"` seguindo padrão existente.

---

## 2. Schema — Alterações no Banco de Dados

### 2.1 Campos novos em `membros`

| Campo | Tipo | Nullable | Default | Descrição |
|-------|------|----------|---------|-----------|
| `salario` | `DECIMAL(12,2)` | YES | NULL | Salário atual R$/mês |
| `data_admissao` | `DATE` | YES | NULL | Data de admissão |
| `banco_horas` | `DECIMAL(8,2)` | YES | 0 | Saldo atual banco de horas |

### 2.2 Tabela `membro_salarios`

Histórico salarial — insert automático ao atualizar `membros.salario`.

```sql
CREATE TABLE membro_salarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(12,2) NOT NULL,
    data_vigencia DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_membro_salarios_membro ON membro_salarios(membro_id, data_vigencia);
```

### 2.3 Tabela `membro_banco_horas`

Histórico banco de horas — insert automático ao atualizar `membros.banco_horas`.

```sql
CREATE TABLE membro_banco_horas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(8,2) NOT NULL,
    data_registro TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_membro_banco_horas_membro ON membro_banco_horas(membro_id, data_registro);
```

---

## 3. Backend API

### 3.1 Endpoints novos — Investimentos

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/api/v1/equipes/{id}/investimentos` | Dashboard completo: sumário + lista membros |
| `GET` | `/api/v1/equipes/{id}/investimentos/gastos-mensais` | 12 meses custo total mensal |

### 3.2 Endpoints novos — Membro (campos financeiros)

| Método | Rota | Descrição |
|--------|------|-----------|
| `PUT` | `/api/v1/membros/{id}/salario` | Atualiza salário + registra histórico |
| `PUT` | `/api/v1/membros/{id}/banco-horas` | Atualiza banco horas + registra histórico |
| `PUT` | `/api/v1/membros/{id}/data-admissao` | Atualiza data admissão |
| `GET` | `/api/v1/membros/{id}/salario/historico` | Lista histórico salarial |
| `GET` | `/api/v1/membros/{id}/banco-horas/historico` | Lista histórico banco horas |
| `GET` | `/api/v1/membros/{id}/alocacoes-projetos` | Projetos + % alocação até final do ano |

### 3.3 Response — `GET /equipes/{id}/investimentos`

```json
{
  "equipe": { "id": "uuid", "nome": "Devops RM" },
  "sumario": {
    "custo_mensal_total": 45000.00,
    "total_membros": 6,
    "tempo_casa_medio_meses": 24,
    "banco_horas_total": 526.5
  },
  "membros": [
    {
      "id": "uuid",
      "nome": "João Silva",
      "avatar_url": "https://...",
      "salario": 12000.00,
      "data_admissao": "2024-03-15",
      "tempo_casa_meses": 29,
      "banco_horas": 340.0,
      "cargo": "desenvolvedor",
      "top_produtos": ["Portal", "App Mobile", "API Gateway"]
    }
  ]
}
```

- `membros` ordenados por `salario DESC`
- `tempo_casa_meses` calculado no backend: `data_admissao` → data atual
- `top_produtos` — top 3 produtos com mais cards atribuídos ao membro (query `tarefa_produtos` + `tarefas.responsavel_id`)

### 3.4 Response — `GET /equipes/{id}/investimentos/gastos-mensais`

```json
{
  "ano": 2026,
  "meses": [
    { "mes": 1, "custo_total": 48000.00 },
    { "mes": 2, "custo_total": 48000.00 },
    { "mes": 3, "custo_total": 36000.00 }
  ]
}
```

**Cálculo por mês:**
1. Lista membros da equipe
2. Para cada mês (1-12), verifica quem estava ativo:
   - Membro com `data_admissao` posterior ao mês → não conta
   - Membro com `data_desligamento` anterior ao mês → não conta
3. Busca salário vigente naquele mês via `membro_salarios` (registro com `data_vigencia <= último dia do mês`, mais recente)
4. Se sem histórico, usa `membros.salario` atual
5. Soma todos salários ativos = custo do mês

### 3.5 Response — `GET /membros/{id}/alocacoes-projetos`

```json
{
  "projetos": [
    {
      "apelido": "Portal Web",
      "chave_jira": "PROJ-123",
      "percentual_alocacao": 40.0
    },
    {
      "apelido": "App Mobile",
      "chave_jira": "PROJ-456",
      "percentual_alocacao": 35.0
    }
  ]
}
```

Dados vindos do módulo de alocação existente (`AllocationRepository`). Percentual = horas no projeto / total horas alocadas × 100.

### 3.6 Service Layer

Novo `InvestimentoService`:

```go
type InvestimentoService struct {
    equipeRepo     EquipeStore
    membroRepo     *repository.MembroRepository
    allocationRepo *repository.AllocationRepository
    tarefaRepo     *repository.TarefaRepository
    logger         *zap.Logger
}
```

**Métodos:**
- `GetDashboard(ctx, equipeID) → InvestimentoDashboard`
- `GetGastosMensais(ctx, equipeID, ano) → []GastoMensal`
- `UpdateSalario(ctx, membroID, valor) error` — atualiza campo + insere histórico
- `UpdateBancoHoras(ctx, membroID, valor) error` — atualiza campo + insere histórico
- `UpdateDataAdmissao(ctx, membroID, data) error`
- `GetHistoricoSalario(ctx, membroID) → []SalarioHistorico`
- `GetHistoricoBancoHoras(ctx, membroID) → []BancoHorasHistorico`
- `GetAlocacoesProjetos(ctx, membroID) → []ProjetoAlocacao`

---

## 4. Frontend — Página Investimentos

### 4.1 Filtro de Equipe + Avatares

Topo da página:
- Dropdown seleção de equipe (carrega equipes do usuário)
- Ao selecionar: mostra avatares circulares dos membros ao lado (padrão `.member-avatar`)
- Tooltip com nome no hover sobre cada avatar
- Click no avatar scrolla pra pessoa na tabela

### 4.2 Header — Nome da Equipe

Nome completo da equipe em destaque (ex: "Devops RM") — tipografia grande, cor accent.

### 4.3 Sumário + Gráfico (lado a lado)

Layout flex/grid — sumário cards à esquerda, gráfico à direita.

**Cards sumário** (padrão `.review-stat-card` do Sprint Review):

| Card | Valor | Label |
|------|-------|-------|
| Investimento/Mês | R$ 45.000 | custo mensal total |
| Membros | 6 | total ativos |
| Tempo Médio | 2a 0m | média tempo de casa |
| Banco de Horas | 526,5h | total acumulado |

Número grande em destaque, label menor abaixo.

**Gráfico de linha** — SVG custom:
- Eixo X: Jan-Dez (12 meses)
- Eixo Y: R$ (custo total mensal)
- Tooltip no hover mostra valor exato
- Meses passados e atual: valores reais do endpoint `gastos-mensais`
- Meses futuros: projeção tracejada — frontend repete `custo_mensal_total` do sumário (sem endpoint extra)

### 4.4 Tabela de Membros

Abaixo do sumário/gráfico. Ordenado por salário desc.

| Coluna | Conteúdo | Formato |
|--------|----------|---------|
| Foto | Avatar circular (foto ou iniciais) | `.member-avatar` |
| Nome | Nome completo | texto |
| R$/Mês | Salário | `R$ 12.000,00` |
| Tempo de Casa | Calculado frontend | `2a 5m` |
| Banco de Horas | Saldo | `340h` |
| Atuação | Top 3 produtos | chips/tags |

Click na linha → abre modal detalhe.

### 4.5 Modal Detalhe da Pessoa

Modal overlay (padrão `.modal-overlay` existente), tamanho maior (`max-width: 720px`).

**Conteúdo:**

1. **Header:** Avatar + Nome + Cargo + botão voltar/fechar

2. **Projetos & Alocações:**
   - Lista de projetos com apelido em destaque
   - Chave JIRA abaixo, menor e opaca (`opacity: 0.5`)
   - Barra de % alocação visual (barra colorida proporcional)
   - Período: até final do ano corrente

3. **Gráficos histórico (lado a lado):**
   - **Histórico Salarial** — gráfico de linha SVG, eixo X = datas vigência, eixo Y = R$
   - **Banco de Horas** — gráfico de linha SVG, eixo X = datas registro (por ocorrência), eixo Y = horas

---

## 5. Sub-projetos

| Sub-projeto | Escopo | Estimativa | Dependência |
|-------------|--------|:----------:|:-----------:|
| **SP1: Schema + Backend** | Migrations, repository, service, handler, testes | ~4-5h | Nenhuma |
| **SP2: Frontend** | Menu, página, gráficos, modal, integração | ~5-6h | SP1 |

**Ordem:** SP1 primeiro → SP2.

Cada sub-projeto gera plan → implementação própria.

---

## 6. Decisões Registradas

| Decisão | Escolha | Alternativa descartada |
|---------|---------|----------------------|
| Nome do módulo | Investimentos | Painel Financeiro, Gestão de Pessoas, Custos |
| Agrupamento menu | Estrutura (Equipes + Investimentos) | Manter Equipes separado |
| Salário | Valor único + histórico automático | Só valor único sem histórico |
| Banco de Horas | Input manual + histórico por ocorrência | Calculado de JIRA, importado |
| Horas extras impacto financeiro | Sem impacto — banco de horas é saldo em horas | Somar ao custo mensal |
| Gráfico gastos | Custo total mensal (não acumulado) | Acumulado no ano |
| Acesso | Mesmo controle de equipes | Permissão separada, cargo-based |
| Alocação projetos | Dados existentes read-only | Cadastro manual novo |
| Gráficos | SVG custom (sem lib externa) | Recharts, Chart.js |
