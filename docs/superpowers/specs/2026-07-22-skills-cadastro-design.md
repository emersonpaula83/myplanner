# Skills Cadastro — Design Spec

## Objetivo

Cadastro de skills (tags técnicas) no módulo Pessoas. Catálogo global de skills com associação N:N a membros. API standalone reutilizável por outros módulos (ex: equalizador).

## Escopo

- Catálogo global de skills (CRUD)
- Associação binária membro↔skill (sem nível de proficiência)
- Autocomplete com sugestão de skills existentes ao adicionar
- Criação inline de nova skill quando não existe no catálogo
- Exibição e gestão apenas na página de detalhe do membro
- API projetada para reuso cross-módulo

## Fora de Escopo

- Níveis de proficiência (junior/pleno/senior)
- Exibição de skills na listagem de membros
- Filtro/busca por skill na lista de membros
- Tela separada de administração do catálogo
- Categorias ou agrupamentos de skills

## Banco de Dados

Migration file: `000013_skills.up.sql` / `000013_skills.down.sql`.

```sql
CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT skills_nome_unique UNIQUE (nome)
);

CREATE TABLE membro_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT membro_skills_unique UNIQUE (membro_id, skill_id)
);
```

- `nome` com UNIQUE impede duplicatas no catálogo
- `ON DELETE CASCADE` em ambos FKs
- Sem `updated_at` no join (relação binária, não atualiza)

## Backend

### Domain

Arquivo: `backend/internal/domain/skill.go`

```go
type Skill struct {
    ID        uuid.UUID `json:"id"`
    Nome      string    `json:"nome"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Repository

Arquivo: `backend/internal/repository/skill.go`

Construtor: `NewSkillRepository(pool *pgxpool.Pool) *SkillRepository`

Métodos:
- `List(ctx context.Context, query string) ([]domain.Skill, error)` — filtra por `ILIKE '%query%'` no nome. Query vazia retorna todas. Ordenado por `nome ASC`.
- `GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error)` — retorna skill ou erro se não encontrada.
- `Create(ctx context.Context, nome string) (*domain.Skill, error)` — insere nova skill. Usa `ON CONFLICT (nome) DO NOTHING` + `RETURNING` ou fallback select pra retornar existente se duplicada (case-insensitive via `LOWER(nome)`).
- `Delete(ctx context.Context, id uuid.UUID) error` — deleta skill e cascadeia remoção das associações.
- `GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error)` — retorna skills associadas ao membro, ordenado por `nome ASC`.
- `AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error` — insere em `membro_skills`. `ON CONFLICT DO NOTHING` pra idempotência.
- `RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error` — deleta de `membro_skills`.

### Handler

Arquivo: `backend/internal/handler/skill.go`

Interface `SkillStore` com todos métodos do repository.

Construtor: `NewSkillHandler(store SkillStore, logger *zap.Logger) *SkillHandler`

### Endpoints

Catálogo global (standalone, reutilizável):

| Método | Rota | Handler | Descrição |
|--------|------|---------|-----------|
| GET | `/api/v1/skills?q=go` | `List` | Autocomplete/busca de skills |
| POST | `/api/v1/skills` | `Create` | Criar nova skill `{nome: "golang"}` |
| DELETE | `/api/v1/skills/{id}` | `Delete` | Deletar skill do catálogo |

Skills do membro (associação):

| Método | Rota | Handler | Descrição |
|--------|------|---------|-----------|
| GET | `/api/v1/membros/{id}/skills` | `GetMembroSkills` | Listar skills do membro |
| POST | `/api/v1/membros/{id}/skills` | `AddMembroSkill` | Associar skill `{skill_id: "..."}` |
| DELETE | `/api/v1/membros/{id}/skills/{skillId}` | `RemoveMembroSkill` | Desassociar skill |

### Validações

- `POST /api/v1/skills`: `nome` obrigatório, max 100 chars, trimmed
- `POST /api/v1/membros/{id}/skills`: `skill_id` obrigatório, UUID válido
- Duplicata no catálogo: retorna skill existente (200), não erro
- Duplicata na associação: idempotente (200), não erro

### Wiring

Em `main.go`:
- Instanciar `SkillRepository` com pool
- Instanciar `SkillHandler` com repository + logger
- Registrar rotas do catálogo sob `r.Route("/api/v1/skills", ...)`
- Registrar rotas de membro-skills sob grupo existente de membros

## Frontend

Tudo em `frontend/index.html`, na página `page-membro-detail`.

### Localização

Seção de skills renderizada abaixo de nome/email/team-badge e acima dos stats cards. Posicionada dentro de `loadMembroDetail()` / `renderMembroDetail()`.

### Exibição

- Skills como badges pill inline: `background: var(--accent-light); color: var(--accent); border-radius: 12px; padding: 2px 10px; font-size: 12px;`
- Cada badge mostra nome da skill + botão "×" pra remover
- Se sem skills: texto discreto "Nenhuma skill cadastrada"
- Botão "+" após as badges pra abrir input de adição

### Interação de Adição

1. Clicar "+" → mostra input text com placeholder "Adicionar skill..."
2. Digitar → debounce 300ms → `GET /api/v1/skills?q={input}`
3. Dropdown de sugestões abaixo do input (posição absoluta)
4. Selecionar sugestão → `POST /api/v1/membros/{id}/skills` com `skill_id`
5. Digitar nome novo + Enter (sem match no dropdown) → `POST /api/v1/skills` pra criar skill, depois `POST /api/v1/membros/{id}/skills` pra associar
6. Após associação → recarrega lista de skills do membro, fecha input

### Interação de Remoção

- Clicar "×" na badge → `DELETE /api/v1/membros/{id}/skills/{skillId}`
- Sem confirmação (ação leve, reversível)
- Após remoção → recarrega lista de skills do membro

### Funções JS

- `loadMembroSkills(membroId)` — carrega e renderiza badges
- `renderMembroSkills(skills, membroId)` — gera HTML das badges
- `showSkillInput(membroId)` — mostra input com autocomplete
- `searchSkills(query)` — busca catálogo pra autocomplete
- `addSkillToMembro(membroId, skillId)` — associa skill existente
- `createAndAddSkill(membroId, nome)` — cria skill + associa
- `removeSkillFromMembro(membroId, skillId)` — desassocia

## Testes

### Backend

- `backend/internal/handler/skill_test.go` — testes de handler via httptest
  - List com e sem query param
  - Create com nome válido, nome vazio, nome duplicado
  - Delete skill existente e inexistente
  - GetMembroSkills
  - AddMembroSkill com skill_id válido, vazio, duplicado
  - RemoveMembroSkill

### Frontend

- Teste manual no browser: adicionar, remover, autocomplete, criar nova skill
