# Alocação de Projetos — Melhorias Visuais

## Objetivo

Melhorar experiência visual do módulo de Alocação: deadline no Gantt, responsável no modal, badges de alerta, fotos dos devs, reorganização do menu.

## 1. Sidebar — Mover Menu

"Alocação" sai do grupo "Relatórios" e vai para grupo "Projetos" no sidebar. Sem mudança de rota, ID de página, ou funcionalidade — só reposicionar o `<li>` no HTML.

## 2. Backend — Dados Adicionais

### Novo método `GetEpicByID`

Query direta por ID do épico com JOIN em `membros` para responsável:

```sql
SELECT
    e.id, e.numero_ticket, e.resumo, e.apelido,
    e.data_limite::timestamptz, e.prioridade,
    COALESCE(e.tipo_demanda, ...),
    (SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada'))::int,
    (SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0)::int,
    COALESCE((SELECT SUM(c.estimativa_tempo) ...), 0)::float8 / 3600.0,
    COALESCE((SELECT SUM(c.estimativa_tempo) ... AND s.estado IN ('active', 'future')), 0)::float8 / 3600.0,
    m.id, m.nome, m.avatar_url, m.cargo
FROM tarefas e
LEFT JOIN membros m ON m.id = e.responsavel_id
WHERE e.id = $1
```

Retorna `*EpicAllocationRow` com campos extras para responsável.

### Tipo `EpicAllocationRow` — campos adicionais

```go
ResponsavelID     *uuid.UUID
ResponsavelNome   *string
ResponsavelAvatar *string
ResponsavelCargo  *string
```

### `GetEpicPeople` — adicionar avatar

```sql
SELECT m.id, m.nome, m.avatar_url, COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0
```

`PersonAllocationRow` ganha `AvatarURL *string`.

### `GetEpicTasks` — adicionar avatar do responsável

```sql
SELECT ..., m.avatar_url
FROM tarefas t
LEFT JOIN membros m ON m.id = t.responsavel_id
```

`TaskAllocationRow` ganha `ResponsavelAvatar *string`.

### Structs de serviço atualizados

```go
type ProjectAllocation struct {
    // ... campos existentes ...
    ResponsavelNome   *string `json:"responsavel_nome"`
    ResponsavelAvatar *string `json:"responsavel_avatar"`
    ResponsavelCargo  *string `json:"responsavel_cargo"`
}

type PersonAllocation struct {
    // ... campos existentes ...
    AvatarURL string `json:"avatar_url"`
}

type TaskAllocation struct {
    // ... campos existentes ...
    ResponsavelAvatar *string `json:"responsavel_avatar"`
}
```

## 3. Gantt — Linha de Deadline

Se `detail.epic.data_limite` está definida, renderizar linha vertical vermelha tracejada na posição correspondente no timeline do Gantt.

### Posicionamento

Mesma lógica das barras:
```js
var deadlineDate = new Date(detail.epic.data_limite).getTime();
var deadlineLeft = (deadlineDate - yearStart) / yearRange * 100;
```

Se `deadlineLeft` entre 0-100%, renderizar:
```html
<div class="alloc-gantt-deadline" style="left:{pct}%" title="Data Limite: {dd/mm/yyyy}">
    <div class="alloc-gantt-deadline-label">Limite: {dd/mm/yyyy}</div>
</div>
```

### CSS

```css
.alloc-gantt-deadline {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 2px;
    border-left: 2px dashed #ef4444;
    z-index: 2;
}
.alloc-gantt-deadline-label {
    position: absolute;
    top: -18px;
    left: 4px;
    font-size: 10px;
    color: #ef4444;
    white-space: nowrap;
}
```

## 4. Modal Header — Responsável e Badges

### Responsável do projeto

Ao lado do título, se `epic.responsavel_nome` existe:

```html
<div class="alloc-responsavel">
    <img class="alloc-avatar" src="{avatar_url}" title="{nome_completo}">
    <span>{primeiro_nome}</span>
    <span class="alloc-cargo">{cargo_formatado}</span>
</div>
```

Foto circular 24px. Primeiro nome via `nome.split(' ')[0]`. Tooltip mostra nome completo. Cargo formatado (ex: "coordenador_desenvolvimento" → "Coord. Dev").

Sem avatar_url: placeholder com iniciais (círculo cinza + letra).

### Badge GDPTC

Se `epic.is_gdptc`:
```html
<span class="alloc-badge alloc-badge-gdptc">★ Projeto do Portfólio Unificado</span>
```

Cor accent, junto aos outros badges no header.

### Badges de Deadline

Lógica (calculada no frontend):

```js
var now = new Date();
var hasPending = (detail.nao_alocadas && detail.nao_alocadas.length > 0) ||
                 (detail.parciais && detail.parciais.length > 0);

if (epic.data_limite && hasPending) {
    var limite = new Date(epic.data_limite);
    var diasRestantes = Math.ceil((limite - now) / 86400000);

    if (diasRestantes < 0) {
        // Badge vermelho: "Em Atraso - Data Limite: dd/mm/yyyy"
    } else if (diasRestantes <= 30) {
        // Badge amarelo: "Data limite próxima - dd/mm/yyyy"
    }
}
```

CSS:
- `.alloc-badge-atrasado` — `background: #ef4444; color: white`
- `.alloc-badge-proximo` — `background: #eab308; color: #1a1a1a`

## 5. Fotos + Primeiro Nome nas Listas

### Helper JS

```js
function firstName(nome) {
    if (!nome) return '--';
    return nome.split(' ')[0];
}
```

### Tabela "Equipe Envolvida"

Coluna "Nome" muda para: foto circular 20px + primeiro nome. `title` no `<td>` = nome completo.

```html
<td title="{nome_completo}">
    <img class="alloc-avatar-sm" src="{avatar_url}"> {primeiro_nome}
</td>
```

Sem avatar: placeholder com inicial.

### Linhas de Tarefas (editáveis e readonly)

Onde mostra responsável: foto circular 16px + primeiro nome. `title` = nome completo.

Para tarefas editáveis: o select de pessoa continua com nome completo (precisa para diferenciar homônimos).

### CSS avatares

```css
.alloc-avatar {
    width: 24px; height: 24px;
    border-radius: 50%;
    object-fit: cover;
    vertical-align: middle;
}
.alloc-avatar-sm {
    width: 20px; height: 20px;
    border-radius: 50%;
    object-fit: cover;
    vertical-align: middle;
    margin-right: 4px;
}
.alloc-avatar-xs {
    width: 16px; height: 16px;
    border-radius: 50%;
    object-fit: cover;
    vertical-align: middle;
    margin-right: 3px;
}
.alloc-avatar-placeholder {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--border);
    color: var(--text-secondary);
    font-size: 10px;
    font-weight: 600;
    vertical-align: middle;
}
```

## 6. Boxes na Tela Principal — Badges de Deadline

Nos boxes (renderAllocationBoxes), mesma lógica de badges:

- Amarelo "Limite próximo - dd/mm/yyyy" se ≤30 dias + `tarefas_sem_estimativa > 0` ou `pct_planejado < 100`
- Vermelho "Em Atraso - dd/mm/yyyy" se passado + condição acima

Posição: após o badge de status existente (Planejado/Em Planejamento/Não Planejado).

## Formatação de Cargo

Map de valores do banco para labels curtos:
```js
var cargoLabels = {
    'coordenador_desenvolvimento': 'Coord. Dev',
    'po_produto': 'PO',
    'gerente_tecnologia': 'Ger. Tecnologia',
    'gerente_executivo': 'Ger. Executivo',
    'scrum_master': 'Scrum Master',
    'agile_master': 'Agile Master',
    'desenvolvedor': 'Dev'
};
```

## Restrições Globais

- Frontend: `var`/`function` only, NO ES6+. XSS: `esc()` para texto, `escAttr()` para atributos
- CSS custom properties: `--surface`, `--text-primary`, `--accent`, `--border`, `--text-secondary`
- Dark mode: `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]` + `:root[data-theme="light"]`
- Sem commits automáticos — mudanças ficam unstaged
