# Timeline de Capacidade — Design Spec

## Visão Geral

Relatório visual tipo Gantt Chart exibindo projetos (épicos do JIRA) de uma equipe ao longo do ano, com overlay de capacidade mensal mostrando sobre/sub-utilização da equipe. Inclui análise por IA (Gemini) para meses com desvio de capacidade.

**Terminologia:** No tCloud Planner, "Projeto" = Épico do JIRA. A tabela `projetos` (JIRA Projects/workspaces) é conceito interno, não exposto no UI.

## Requisitos Funcionais

### Filtros

- **Equipe**: seleção única, obrigatório. Corresponde ao campo `team` nas tarefas/membros.
- **Ano**: dropdown com ano atual selecionado por padrão. Opções: ano atual ± 2 anos.

### Gantt Chart

**Eixo X (horizontal):** Timeline do ano dividida em 4 Quarters (Q1=jan-mar, Q2=abr-jun, Q3=jul-set, Q4=out-dez). Cada quarter subdivido em 3 meses com linhas verticais de menor destaque.

**Eixo Y (vertical):** Apelidos dos projetos empilhados verticalmente. Ordenação:
1. Em execução (status "Em Andamento" ou "Desenvolvimento") primeiro
2. Backlog com `data_limite` ainda no ano do filtro, ordenado por `data_limite` ASC

**Barras horizontais:**
- Início: `data_inicio_execucao` do épico (sync do JIRA ou edição manual)
- Fim: `data_limite` do épico
- Dentro da barra: resumo do épico (`resumo`) em tipografia com opacidade baixa. Se não couber, truncar com "..." e exibir resumo completo no hover.
- Estrela pequena na barra se épico tem parent com `numero_ticket LIKE 'GDPTC-%'` (projeto CI - Gestão de Portfolio). Hover na estrela: "Projeto CI - Gestão de Portfolio".

**Hover no nome do projeto (eixo Y):**
- Total de dias estimados: soma de `estimativa_tempo` dos cards filhos da equipe / 3600 / 8
- Data Limite do épico
- Tipo de Demanda: Meta, Compromisso ou Iniciativa (`tipo_demanda`)

### Filtro de Inclusão de Projetos

Para um épico aparecer no Gantt:
1. `tipo = 'Épico'` na tabela `tarefas`
2. Possui cards filhos (`parent_id` aponta pro épico) com `team = equipe_selecionada`
3. Status (campo `status` da tarefa):
   - "Em Andamento" ou "Desenvolvimento" → sempre exibido
   - "Backlog" → exibido somente se `data_limite` cai no ano do filtro
   - Qualquer outro status (ex: "Concluído", "Cancelado") → não exibido

**Nota:** Nomes de status são literais do JIRA. Se o workspace usar nomes diferentes, ajustar os valores de filtro.

### Fallback do Apelido

Se `apelido` for NULL, exibir `numero_ticket` do épico no eixo Y (ex: "BACK-142"). Apelido é sempre preferido quando preenchido.

### Capacity Overlay

Faixas verticais mensais sobre o Gantt, uma por mês:

**Cálculo por mês:**
```
horas_disponiveis = membros_ativos × dias_uteis_no_mes × 8
                    - soma(dias_ausencia × 8) por membro

horas_estimadas  = Σ (horas_projeto × dias_projeto_no_mes / dias_projeto_total)
                   para cada projeto que intersecta o mês

horas_projeto    = soma(estimativa_tempo dos cards filhos da equipe) / 3600

percentual_delta = ((horas_estimadas - horas_disponiveis) / horas_disponiveis) × 100
```

**Distribuição linear:** Horas de um projeto são distribuídas linearmente de `data_inicio_execucao` até `data_limite`. Se intersecta um mês parcialmente, proporção baseada nos dias do projeto dentro daquele mês.

**Visual:**
- `percentual_delta > 0`: faixa vermelha com opacidade baixa, label "+X%" em vermelho
- `percentual_delta < 0`: faixa verde com opacidade baixa, label "-X%" em verde
- Opacidade não compromete visibilidade dos elementos do Gantt

**Dias úteis:** segunda a sexta, sem feriados (v1). Feriados configuráveis em versão futura.

**Ausências consideradas:** dayoff, ferias, licenca_medica, licenca_paternidade, licenca_maternidade (tabela `disponibilidade`).

### Análise por IA

Ao passar o mouse na faixa de capacity de um mês, exibir botão "Analisar". Ao clicar:
1. Backend monta contexto: dados de capacity do mês, ausências, projetos ativos
2. Envia para Gemini API com prompt contextualizado
3. Retorna análise + recomendação em popup

Exemplo de resposta: "João estará de férias e Maria estará de licença maternidade reduzindo o capacity total em -40%. Recomendação: replanejamento."

### Gerenciamento de Metadados de Projeto (Menu Projetos)

No menu "Projetos", para cada épico:
- **Apelido**: campo texto, máximo 15 caracteres, armazenado em uppercase. Exibido no eixo Y do Gantt.
- **Data Início Execução**: campo data/hora. Sync automático do JIRA (changelog de transição para Em Andamento/Desenvolvimento) quando disponível, edição manual como fallback.

## Schema Changes

Três campos novos na tabela `tarefas`:

```sql
ALTER TABLE tarefas ADD COLUMN parent_id UUID REFERENCES tarefas(id) ON DELETE SET NULL;
ALTER TABLE tarefas ADD COLUMN apelido VARCHAR(15);
ALTER TABLE tarefas ADD COLUMN data_inicio_execucao TIMESTAMPTZ;

CREATE INDEX idx_tarefas_parent ON tarefas(parent_id);
CREATE INDEX idx_tarefas_tipo_team_epico ON tarefas(tipo, team) WHERE tipo = 'Épico';
```

- **`parent_id`**: referência pai-filho genérica. Cards apontam pro épico. Épico pode apontar pro card GDPTC-*.
- **`apelido`**: nome curto uppercase do projeto pra exibição no Gantt.
- **`data_inicio_execucao`**: timestamp de quando o épico entrou em execução.

Domain model — adicionar em `Tarefa` (models.go):

```go
ParentID            *uuid.UUID    `json:"parent_id" db:"parent_id"`
Apelido             *string       `json:"apelido" db:"apelido"`
DataInicioExecucao  *time.Time    `json:"data_inicio_execucao" db:"data_inicio_execucao"`
```

## API Endpoints

### GET /api/v1/timeline-capacidade

**Query params:**
- `equipe` (string, obrigatório): nome da equipe
- `ano` (int, obrigatório): ano do filtro

**Response 200:**

```json
{
  "equipe": "Backend",
  "ano": 2026,
  "projetos": [
    {
      "id": "uuid",
      "apelido": "MIGRAÇÃO DB",
      "resumo": "Migrar banco de dados para nova arquitetura com suporte a multi-tenant",
      "tipo_demanda": "Meta",
      "status": "Em Andamento",
      "data_inicio_execucao": "2026-03-15T00:00:00Z",
      "data_limite": "2026-08-30",
      "total_dias_estimados": 45,
      "projeto_ci": false,
      "projeto_ci_ticket": null
    }
  ],
  "capacidade_mensal": [
    {
      "mes": 1,
      "horas_disponiveis": 640.0,
      "horas_estimadas": 520.0,
      "percentual_delta": -18.75,
      "membros_ausentes": []
    },
    {
      "mes": 7,
      "horas_disponiveis": 480.0,
      "horas_estimadas": 680.0,
      "percentual_delta": 41.67,
      "membros_ausentes": [
        {"nome": "João", "tipo": "ferias", "dias": 15},
        {"nome": "Maria", "tipo": "licenca_maternidade", "dias": 22}
      ]
    }
  ]
}
```

- `percentual_delta` positivo = over capacity (vermelho), negativo = under (verde)
- `total_dias_estimados` = soma `estimativa_tempo` dos cards filhos da equipe ÷ 3600 ÷ 8
- `projeto_ci` = parent com `numero_ticket LIKE 'GDPTC-%'`
- Meses sem projetos ativos: `horas_estimadas = 0`, `percentual_delta` negativo

### PUT /api/v1/projetos/{id}/metadata

**Body:**

```json
{
  "apelido": "MIGRAÇÃO DB",
  "data_inicio_execucao": "2026-03-15T00:00:00Z"
}
```

**Validações:**
- `id` deve ser tarefa com `tipo = 'Épico'`
- `apelido`: max 15 chars, convertido pra uppercase automaticamente
- `data_inicio_execucao`: formato RFC3339

**Response 200:** Tarefa atualizada (campos relevantes).

### GET /api/v1/projetos

**Query params:**
- `equipe` (string, opcional): filtra épicos que têm cards da equipe

**Response 200:** Lista de épicos com campos: id, numero_ticket, resumo, apelido, data_inicio_execucao, data_limite, tipo_demanda, status.

### POST /api/v1/timeline-capacidade/analisar

**Body:**

```json
{
  "equipe": "Backend",
  "ano": 2026,
  "mes": 7
}
```

**Response 200:**

```json
{
  "analise": "João estará de férias (15 dias) e Maria estará de licença maternidade (22 dias úteis) reduzindo o capacity total em -40%. Com 680h estimadas contra 480h disponíveis, a equipe está sobrecarregada em +41.67%. Recomendação: replanejamento dos projetos MIGRAÇÃO DB e API GATEWAY para o Q4, ou alocação temporária de recursos de outra equipe."
}
```

**Timeout:** 30s. Retry: 1x em caso de erro transiente.

## Arquitetura de Componentes

```
cmd/api/main.go ─── routes
│
├── handler/timeline.go
│   ├── GET  /timeline-capacidade     → ListTimeline()
│   ├── PUT  /projetos/{id}/metadata  → UpdateProjetoMetadata()
│   ├── GET  /projetos                → ListProjetos()
│   └── POST /timeline-capacidade/analisar → AnalisarCapacidade()
│
├── handler/timeline_calc.go
│   └── CalcularCapacidadeMensal()    ← função pura, testável
│   └── DistribuirHorasPorMes()       ← função pura, testável
│
├── repository/timeline.go
│   ├── BuscarEpicosEquipe()
│   ├── BuscarCardsFilhosEquipe()
│   ├── BuscarMembrosEquipe()
│   ├── BuscarAusenciasEquipe()
│   ├── AtualizarMetadataProjeto()
│   └── VerificarProjetoCI()
│
├── service/gemini.go
│   ├── AnalisadorCapacidade (interface)
│   └── GeminiAnalyzer (implementação)
│
├── domain/timeline.go              ← tipos específicos
└── domain/models.go                ← Tarefa atualizada
```

## Gemini Integration

**Interface:**

```go
type AnalisadorCapacidade interface {
    Analisar(ctx context.Context, req AnaliseRequest) (string, error)
}

type AnaliseRequest struct {
    Equipe           string
    Ano              int
    Mes              int
    HorasDisponiveis float64
    HorasEstimadas   float64
    PercentualDelta  float64
    MembrosAusentes  []MembroAusente
    Projetos         []ProjetoResumo
}
```

**Implementação:**
- SDK: `google.golang.org/genai`
- API key via config: `GEMINI_API_KEY` (env var)
- Modelo: `gemini-2.0-flash`
- Prompt em português, contextualizado com dados completos do mês
- Response: diagnóstico breve + recomendação acionável (3-4 frases)
- Timeout: 30s, retry: 1x em erro transiente

**Config (config.go):**

```go
type GeminiConfig struct {
    APIKey string `env:"GEMINI_API_KEY"`
    Model  string `env:"GEMINI_MODEL" envDefault:"gemini-2.0-flash"`
}
```

## Fora do Escopo

- Sync do JIRA (spec assume dados populados via sync ou seed)
- Frontend/UI (protótipo HTML separado)
- Feriados configuráveis (v2)
- Histórico completo de status (apenas `data_inicio_execucao` sincronizado)
