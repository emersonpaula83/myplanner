# Sprint Review Module — Design Spec

## Overview

New "Review" submenu under Sprint, alongside renamed "Acompanhamento" (current sprint detail). Generates a visual sprint review report with stats, pie charts, editable highlights ("destaques"), and task table. Exportable to PDF/image.

## Navigation

- Sprint detail current content renamed to **Acompanhamento**
- New **Review** tab added alongside it
- Tab switching via innerHTML swap inside `#sprints-content` (existing pattern)

## Flow

1. User selects Equipe → Sprint (mandatory)
2. Optional multi-select Produto (Componente) filter — all products shown by default
3. Data loads: header stats, PO info, pie charts, destaques, task table

## Data Source

### Base query

All tasks from selected sprint, excluding `status IN ('Cancelado', 'Rejeitada')`. Products via `tarefa_produtos` JOIN `produtos`.

### Status classification

| Category | Status values |
|---|---|
| Concluídas | `Concluído` |
| Em Andamento | `Desenvolvimento`, `Deploy`, `Code Review`, `Teste`, `Validação do Solicitante` |
| Não iniciadas | `Aberto`, `Backlog`, `Avaliação` |

### Type classification

| Category | Rule |
|---|---|
| Bugs e Incidentes (Manutenção) | `tipo IN ('Bug', 'Incidente')` |
| Novos Projetos | Any ancestor in `parent_id` chain has `numero_ticket LIKE 'GDPTC-%'` |
| Melhorias | `tipo IN ('Melhoria', 'História')` AND NOT Novos Projetos |
| Outros | `tipo = 'Tarefa'` or any remaining type |

### Planejada detection

Unplanned = `t.data_entrada_sprint > s.data_inicio OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)`. Planejada = NOT unplanned. Same logic used throughout codebase (`GetUnplannedStats`, `GetDisclaimerTasks`, etc.).

### GDPTC ancestor detection

Recursive CTE walking `parent_id` chain:

```sql
WITH RECURSIVE ancestors AS (
    SELECT id, parent_id, numero_ticket FROM tarefas WHERE id = $1
    UNION ALL
    SELECT t.id, t.parent_id, t.numero_ticket
    FROM tarefas t JOIN ancestors a ON t.id = a.parent_id
)
SELECT EXISTS(SELECT 1 FROM ancestors WHERE numero_ticket LIKE 'GDPTC-%')
```

Called per task with `parent_id NOT NULL` that isn't Bug/Incidente. Sprint has ~200 tasks max, chain depth 2-3 levels — performance acceptable.

## API

### Review data endpoint

`GET /api/sprints/{id}/review?equipe_id=X&produtos=Y,Z` — `equipe_id` required, `produtos` optional comma-separated UUIDs

Response:

```json
{
  "pos": [{"nome": "João Silva", "produtos": ["Produto A", "Produto B"]}],
  "stats": {
    "total": 85,
    "concluidas": 60,
    "em_andamento": 15,
    "planejadas_concluidas": 50,
    "bugs_incidentes": 10,
    "melhorias_inovacoes": 20,
    "outros": 15,
    "detalhes": {
      "em_andamento": {"desenvolvimento": 5, "deploy": 3, "code_review": 4, "teste": 2, "validacao": 1},
      "bugs_incidentes": {"bugs": 7, "incidentes": 3},
      "melhorias_inovacoes": {"portfolio_gdptc": 12, "melhorias": 5, "historias": 3}
    }
  },
  "grafico_produtos": [
    {"produto": "Produto A", "total": 30, "concluidas": 25}
  ],
  "grafico_categorias": [
    {"categoria": "manutencao", "total": 10},
    {"categoria": "novos_projetos", "total": 20},
    {"categoria": "melhorias", "total": 15},
    {"categoria": "outros", "total": 10}
  ],
  "grafico_planejamento": {
    "planejadas": 60,
    "nao_planejadas": 25,
    "nao_planejadas_bugs": 8,
    "nao_planejadas_outras": 17
  },
  "tarefas": [
    {
      "numero_ticket": "PROJ-123",
      "produto": "Produto A",
      "resumo": "Implementar feature X",
      "relator": "Maria Santos"
    }
  ]
}
```

### Destaques CRUD

- `GET /api/sprints/{sprintId}/review/destaques?equipe_id=X` — list
- `POST /api/sprints/{sprintId}/review/destaques` — create `{equipe_id, produto_id, titulo, descricao, link?}`
- `PUT /api/destaques/{id}` — update
- `DELETE /api/destaques/{id}` — delete

## Database

### New table: `sprint_review_destaques`

```sql
CREATE TABLE sprint_review_destaques (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id UUID NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    produto_id UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
    titulo VARCHAR(200) NOT NULL,
    descricao TEXT NOT NULL,
    link VARCHAR(500),
    ordem INT NOT NULL DEFAULT 0,
    criado_em TIMESTAMP NOT NULL DEFAULT NOW(),
    atualizado_em TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_destaques_sprint_equipe ON sprint_review_destaques(sprint_id, equipe_id);
```

## PO Display

Query members with `cargo = 'po_produto'` for selected equipe, joined via `membro_produtos`. When product filter active, show only POs of filtered products. When no filter, show all POs.

```sql
SELECT m.nome, ARRAY_AGG(DISTINCT p.nome) AS produtos
FROM membros m
JOIN membro_produtos mp ON mp.membro_id = m.id
JOIN produtos p ON p.id = mp.produto_id
WHERE m.equipe_id = $1 AND m.cargo = 'po_produto'
  AND ($2::uuid[] IS NULL OR p.id = ANY($2))
GROUP BY m.id, m.nome
```

## Frontend Layout

### Header stats cards

Six cards showing percentages with hover tooltips:

| Card | Value | Hover detail |
|---|---|---|
| Concluídas | % concluídas / total | "X de Y tarefas concluídas" |
| Em Andamento | % em andamento / total | Breakdown by status: Desenvolvimento, Deploy, Code Review, Teste, Validação |
| Plan. Concluídas | % planejadas concluídas / planejadas | "X de Y planejadas foram concluídas" |
| Bugs e Incidentes | % bugs+incidentes / total | "Bugs: X, Incidentes: Y" |
| Melhorias e Inovações | % melhorias+inovações / total | "Portfólio (GDPTC): X, Melhorias: Y, Histórias: Z" |
| Outros | % outros / total | "Tarefas: X" |

### Pie charts

Three pie charts reusing existing `drawPieChart` function from disclaimer pizza charts:

1. **Entregas por Produto** — slices per product, values = task count
2. **Categorias** — Manutenção / Novos Projetos / Melhorias / Outros
3. **Planejamento** — Planejadas / Não Planejadas. Hover on "Não Planejadas" shows: "Bugs+Incidentes: X, Outras: Y"

On-slice % labels for slices >10%. Same tooltip pattern as existing pie charts.

### Destaques section

Grouped by product. Each product block has:

- `[+ Adicionar destaque]` button
- List of existing destaques with edit (✏️) and delete (🗑️) buttons
- Each destaque shows: title, description, optional link

**Add/Edit flow**: inline form with fields: título (required, max 200), descrição (required), link (optional, URL validated). Save via POST/PUT.

**Delete flow**: confirmation dialog "Remover destaque?" before DELETE.

**Ordering**: `ordem` field. New destaques get `MAX(ordem) + 1` for same sprint+equipe+produto. No drag-and-drop.

### Tasks table

| Ticket | Produto | Resumo | Relator |
|---|---|---|---|
| PROJ-123 | Produto A | Implementar feature X | Maria Santos |

## Export PDF/Image

### Libraries

`html2canvas` + `jsPDF` minified, inlined as `<script>` tags in `index.html`. ~180KB combined.

### Flow

1. User clicks "Exportar PDF" or "Exportar Imagem"
2. Build temporary div `#review-export-container` with print-friendly layout:
   - Header: sprint name, dates (início — fim), filtered products, PO
   - Stats cards (static, no hover)
   - 3 pie charts (SVG → canvas via html2canvas)
   - Destaques (text only, no action buttons)
   - Tasks table
3. `html2canvas` renders div → canvas (scale: 2 for sharpness, white background forced)
4. **Image**: canvas → `toDataURL('image/png')` → download `review-{sprint_nome}-{data}.png`
5. **PDF**: canvas → jsPDF `addImage()` → download `review-{sprint_nome}-{data}.pdf`
6. Remove temporary div

### Export header

```
Review Sprint 10
01/07/2026 — 15/07/2026
Produtos: Produto A, Produto B
PO: João Silva
```

## Architecture Summary

```
Frontend (index.html)
  ├── Tab: Acompanhamento (existing sprint detail, renamed)
  └── Tab: Review
        ├── Selectors (equipe → sprint → produto filter)
        ├── PO header
        ├── Stats cards with tooltips
        ├── 3 pie charts (reuse drawPieChart)
        ├── Destaques CRUD (inline forms)
        ├── Tasks table
        └── Export buttons (html2canvas + jsPDF)

Backend
  ├── GET /api/sprints/{id}/review?equipe_id&produtos
  │     └── repository: base query + GDPTC recursive CTE + PO query
  ├── GET /api/sprints/{id}/review/destaques?equipe_id
  ├── POST /api/sprints/{id}/review/destaques
  ├── PUT /api/destaques/{id}
  └── DELETE /api/destaques/{id}

Database
  └── sprint_review_destaques (new table)
```

## Non-goals

- Drag-and-drop ordering for destaques
- Server-side PDF generation
- Pagination on tasks table (sprint has ~200 tasks max)
- Dark mode for export (always white background)
