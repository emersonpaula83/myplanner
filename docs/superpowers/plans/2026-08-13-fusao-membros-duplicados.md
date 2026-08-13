# Fusão de Membros Duplicados — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permitir fundir dois registros de `membros` que são a mesma pessoa, deixando uma linha só, com todo o histórico, e fazendo o sync do JIRA reconhecer a conta absorvida para não recriar a duplicata.

**Architecture:** A identidade JIRA passa a ser 1:N — a conta principal continua em `membros.jira_account_id` e as contas absorvidas vão para a tabela nova `membro_jira_contas`. A fusão roda numa transação no repositório: reponta as 11 colunas que apontam para `membros`, grava os campos de RH escolhidos no sobrevivente, move as contas e apaga o perdedor. O sync resolve alias por essa tabela antes do upsert, num único ponto (`UpsertMembro`), porque `SyncService.ensureMember` já cacheia o retorno por `accountID` e o resto do sync resolve responsável e relator por esse cache.

**Tech Stack:** Go 1.x, pgx/v5 + pgxpool, chi v5, zap, PostgreSQL 16, migrations via `golang-migrate` (`./dev.sh migrate`), frontend em HTML/JS puro num arquivo só (`frontend/index.html`).

**Spec:** `docs/superpowers/specs/2026-08-13-fusao-membros-duplicados-design.md`

## Global Constraints

- Migrations numeradas em sequência, com par `.up.sql`/`.down.sql`: a próxima é `000033`.
- Mensagens de erro da API em português, no padrão `respondError(w, status, "texto")` já usado nos handlers.
- Comentário de código em português, no tom do repositório: explica o porquê, não o quê.
- Nada de ORM: SQL escrito à mão com `$1` posicional, `pgxpool`.
- Reponto **sempre antes** do `DELETE`: os filhos de `membros` são `ON DELETE CASCADE` e `tarefas.responsavel_id`/`relator_id` são `ON DELETE SET NULL`.
- `equipe_membros` tem `UNIQUE(membro_id) WHERE data_saida IS NULL` — um vínculo ativo por pessoa.
- `membro_skills` tem `UNIQUE(membro_id, skill_id)` e `membro_produtos` tem `UNIQUE(membro_id, produto_id)`.
- Correção em relação ao texto da spec: `UPDATE` no Postgres **não aceita** `ON CONFLICT`. Onde a spec diz "`UPDATE … ON CONFLICT DO NOTHING`", a implementação usa `UPDATE … WHERE <chave> NOT IN (SELECT <chave> … do sobrevivente)` seguido de `DELETE` do que sobrou.
- Em `MembroFusaoCampos`, campo `nil` significa "não alterar o que o sobrevivente já tem". Não existe forma de limpar um campo pela fusão.
- Fusão não tem desfazer. Nenhuma task implementa reversão.

---

### Task 1: Migration da tabela de contas JIRA

**Files:**
- Create: `backend/migrations/000033_membro_jira_contas.up.sql`
- Create: `backend/migrations/000033_membro_jira_contas.down.sql`

**Interfaces:**
- Consumes: nada.
- Produces: tabela `membro_jira_contas(id, membro_id, fonte_dados_id, jira_account_id, nome_origem, criado_em)` com `UNIQUE(fonte_dados_id, jira_account_id)`, usada pelas Tasks 3, 4 e 5.

- [ ] **Step 1: Escrever a migration up**

`backend/migrations/000033_membro_jira_contas.up.sql`:

```sql
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
```

- [ ] **Step 2: Escrever a migration down**

`backend/migrations/000033_membro_jira_contas.down.sql`:

```sql
DROP TABLE IF EXISTS membro_jira_contas;
```

- [ ] **Step 3: Rodar a migration**

Run: `./dev.sh migrate`
Expected: termina sem erro.

- [ ] **Step 4: Conferir o schema aplicado**

Run: `docker exec myplanner-db-1 psql -U myplanner -d myplanner -c "\d membro_jira_contas"`
Expected: tabela existe, com o índice único `(fonte_dados_id, jira_account_id)` e o índice `idx_membro_jira_contas_membro`.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000033_membro_jira_contas.up.sql backend/migrations/000033_membro_jira_contas.down.sql
git commit -m "feat: tabela membro_jira_contas para contas JIRA absorvidas por fusão"
```

---

### Task 2: Tipos de domínio da fusão

**Files:**
- Create: `backend/internal/domain/fusao.go`

**Interfaces:**
- Consumes: nada.
- Produces: `domain.MembroFusaoLado`, `domain.MembroFusaoPreview`, `domain.MembroFusaoCampos`, `domain.MembroFusaoRequest`, `domain.MembroFusaoResultado` — usados pelas Tasks 3, 5 e 6.

- [ ] **Step 1: Escrever o arquivo de tipos**

`backend/internal/domain/fusao.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// MembroFusaoLado é um dos dois registros da tela de comparação. Além dos
// campos de RH que o operador escolhe, traz contagens para ele enxergar o
// tamanho da operação antes de confirmar.
type MembroFusaoLado struct {
	ID               uuid.UUID  `json:"id"`
	Nome             string     `json:"nome"`
	Email            *string    `json:"email"`
	JiraAccountID    string     `json:"jira_account_id"`
	Cargo            *string    `json:"cargo"`
	Salario          *float64   `json:"salario"`
	Matricula        *string    `json:"matricula"`
	DataAdmissao     *time.Time `json:"data_admissao"`
	UltimoAumento    *time.Time `json:"ultimo_aumento"`
	GestorID         *uuid.UUID `json:"gestor_id"`
	GestorNome       *string    `json:"gestor_nome"`
	Tarefas          int        `json:"tarefas"`
	EquipeNome       *string    `json:"equipe_nome"`
	RegistrosSalario int        `json:"registros_salario"`
	Skills           int        `json:"skills"`
	Ausencias        int        `json:"ausencias"`
}

// MembroFusaoPreview alimenta a tela de comparação. SobreviventeSugeridoID é o
// lado com mais tarefas — é só sugestão, o operador pode inverter.
type MembroFusaoPreview struct {
	Membro                 MembroFusaoLado `json:"membro"`
	Alvo                   MembroFusaoLado `json:"alvo"`
	SobreviventeSugeridoID uuid.UUID       `json:"sobrevivente_sugerido_id"`
}

// MembroFusaoCampos são os valores finais escolhidos pelo operador. Campo nil
// significa "não alterar o que o sobrevivente já tem"; não há como limpar um
// campo pela fusão. Datas em "2006-01-02".
type MembroFusaoCampos struct {
	Cargo         *string    `json:"cargo"`
	Salario       *float64   `json:"salario"`
	Matricula     *string    `json:"matricula"`
	DataAdmissao  *string    `json:"data_admissao"`
	UltimoAumento *string    `json:"ultimo_aumento"`
	GestorID      *uuid.UUID `json:"gestor_id"`
}

type MembroFusaoRequest struct {
	MembroPerdedorID uuid.UUID         `json:"membro_perdedor_id"`
	Campos           MembroFusaoCampos `json:"campos"`
}

// MembroFusaoResultado é o que a tela mostra depois de confirmar.
type MembroFusaoResultado struct {
	TarefasRepontadas     int  `json:"tarefas_repontadas"`
	RegistrosHistorico    int  `json:"registros_historico"`
	VinculoAtivoEncerrado bool `json:"vinculo_ativo_encerrado"`
	ContasAbsorvidas      int  `json:"contas_absorvidas"`
}
```

- [ ] **Step 2: Compilar**

Run: `cd backend && go build ./...`
Expected: sem erro.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/domain/fusao.go
git commit -m "feat: tipos de domínio da fusão de membros"
```

---

### Task 3: Repositório — preview e fusão

**Files:**
- Create: `backend/internal/repository/membro_fusao.go`
- Create: `backend/internal/repository/membro_fusao_test.go`

**Interfaces:**
- Consumes: `domain.MembroFusaoPreview`, `domain.MembroFusaoCampos`, `domain.MembroFusaoResultado` (Task 2); tabela `membro_jira_contas` (Task 1).
- Produces:
  - `func (r *MembroRepository) GetFusaoPreview(ctx context.Context, aID, bID uuid.UUID) (*domain.MembroFusaoPreview, error)`
  - `func (r *MembroRepository) FundirMembros(ctx context.Context, sobreviventeID, perdedorID uuid.UUID, campos domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error)`

  Ambos usados pela Task 5.

- [ ] **Step 1: Escrever o teste de integração que falha**

O valor da funcionalidade está no SQL, então o teste é contra Postgres de verdade, com skip quando não há banco (mesmo espírito de `repository/usuario_saml_test.go`).

`backend/internal/repository/membro_fusao_test.go`:

```go
package repository

import (
	"context"
	"os"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool conecta no Postgres de dev. Sem MYPLANNER_TEST_DB_URL o teste é
// pulado, para `go test ./...` continuar verde em máquina sem banco.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("MYPLANNER_TEST_DB_URL")
	if url == "" {
		t.Skip("MYPLANNER_TEST_DB_URL não definida — teste de integração pulado")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("conectando no banco de teste: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type cenarioFusao struct {
	sobrevivente uuid.UUID
	perdedor     uuid.UUID
	equipe       uuid.UUID
	projeto      uuid.UUID
	fonte        uuid.UUID
}

// semearCenario cria duas pessoas duplicadas, as duas ativas na mesma equipe,
// com tarefas em cada uma — o formato exato do caso que originou a feature.
func semearCenario(t *testing.T, pool *pgxpool.Pool) cenarioFusao {
	t.Helper()
	ctx := context.Background()
	var c cenarioFusao

	if err := pool.QueryRow(ctx, `SELECT id FROM fonte_dados LIMIT 1`).Scan(&c.fonte); err != nil {
		t.Skipf("sem fonte_dados no banco de teste: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM projetos LIMIT 1`).Scan(&c.projeto); err != nil {
		t.Skipf("sem projetos no banco de teste: %v", err)
	}

	sufixo := uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO membros (fonte_dados_id, jira_account_id, nome, cargo)
		VALUES ($1, $2, $3, 'analista_ii') RETURNING id
	`, c.fonte, "test-fusao-sobrevivente-"+sufixo, "Teste Fusao Sobrevivente "+sufixo).Scan(&c.sobrevivente); err != nil {
		t.Fatalf("semeando sobrevivente: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO membros (fonte_dados_id, jira_account_id, nome, matricula)
		VALUES ($1, $2, $3, '9999') RETURNING id
	`, c.fonte, "test-fusao-perdedor-"+sufixo, "Teste Fusao Perdedor "+sufixo).Scan(&c.perdedor); err != nil {
		t.Fatalf("semeando perdedor: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO equipes (nome) VALUES ($1) RETURNING id
	`, "Equipe Teste Fusao "+sufixo).Scan(&c.equipe); err != nil {
		t.Fatalf("semeando equipe: %v", err)
	}
	for _, m := range []uuid.UUID{c.sobrevivente, c.perdedor} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO equipe_membros (equipe_id, membro_id) VALUES ($1, $2)
		`, c.equipe, m); err != nil {
			t.Fatalf("semeando vínculo de equipe: %v", err)
		}
		for i := 0; i < 2; i++ {
			if _, err := pool.Exec(ctx, `
				INSERT INTO tarefas (fonte_dados_id, projeto_id, jira_id, numero_ticket, resumo, tipo, status, data_criacao, responsavel_id)
				VALUES ($1, $2, $3, $4, 'teste fusao', 'Bug', 'Concluído', NOW(), $5)
			`, c.fonte, c.projeto, uuid.NewString(), "TESTF-"+uuid.NewString()[:8], m); err != nil {
				t.Fatalf("semeando tarefa: %v", err)
			}
		}
	}

	t.Cleanup(func() {
		limpar := context.Background()
		pool.Exec(limpar, `DELETE FROM tarefas WHERE responsavel_id = ANY($1::uuid[])`, []uuid.UUID{c.sobrevivente, c.perdedor})
		pool.Exec(limpar, `DELETE FROM membros WHERE id = ANY($1::uuid[])`, []uuid.UUID{c.sobrevivente, c.perdedor})
		pool.Exec(limpar, `DELETE FROM equipes WHERE id = $1`, c.equipe)
	})

	return c
}

func TestFundirMembrosRepontaTudoEApagaPerdedor(t *testing.T) {
	pool := testPool(t)
	repo := NewMembroRepository(pool)
	ctx := context.Background()
	c := semearCenario(t, pool)

	res, err := repo.FundirMembros(ctx, c.sobrevivente, c.perdedor, domain.MembroFusaoCampos{})
	if err != nil {
		t.Fatalf("fundindo: %v", err)
	}

	if res.TarefasRepontadas != 2 {
		t.Errorf("TarefasRepontadas = %d, esperava 2", res.TarefasRepontadas)
	}
	if !res.VinculoAtivoEncerrado {
		t.Error("esperava vínculo ativo duplicado encerrado")
	}

	var tarefasSobrevivente int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tarefas WHERE responsavel_id = $1`, c.sobrevivente).Scan(&tarefasSobrevivente); err != nil {
		t.Fatalf("contando tarefas: %v", err)
	}
	if tarefasSobrevivente != 4 {
		t.Errorf("sobrevivente ficou com %d tarefas, esperava 4", tarefasSobrevivente)
	}

	var vinculos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM equipe_membros WHERE equipe_id = $1 AND data_saida IS NULL`, c.equipe).Scan(&vinculos); err != nil {
		t.Fatalf("contando vínculos: %v", err)
	}
	if vinculos != 1 {
		t.Errorf("equipe ficou com %d vínculos ativos, esperava 1", vinculos)
	}

	var existePerdedor bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM membros WHERE id = $1)`, c.perdedor).Scan(&existePerdedor); err != nil {
		t.Fatalf("verificando perdedor: %v", err)
	}
	if existePerdedor {
		t.Error("perdedor ainda existe em membros")
	}

	var contaAbsorvida uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT membro_id FROM membro_jira_contas WHERE jira_account_id LIKE 'test-fusao-perdedor-%'
	`).Scan(&contaAbsorvida); err != nil {
		t.Fatalf("conta do perdedor não foi registrada: %v", err)
	}
	if contaAbsorvida != c.sobrevivente {
		t.Errorf("conta absorvida aponta para %s, esperava %s", contaAbsorvida, c.sobrevivente)
	}
}

func TestFundirMembrosAplicaCamposEscolhidos(t *testing.T) {
	pool := testPool(t)
	repo := NewMembroRepository(pool)
	ctx := context.Background()
	c := semearCenario(t, pool)

	matricula := "1234"
	cargo := "analista_iii"
	if _, err := repo.FundirMembros(ctx, c.sobrevivente, c.perdedor, domain.MembroFusaoCampos{
		Matricula: &matricula,
		Cargo:     &cargo,
	}); err != nil {
		t.Fatalf("fundindo: %v", err)
	}

	var cargoFinal, matriculaFinal *string
	if err := pool.QueryRow(ctx, `SELECT cargo, matricula FROM membros WHERE id = $1`, c.sobrevivente).Scan(&cargoFinal, &matriculaFinal); err != nil {
		t.Fatalf("lendo sobrevivente: %v", err)
	}
	if cargoFinal == nil || *cargoFinal != "analista_iii" {
		t.Errorf("cargo = %v, esperava analista_iii", cargoFinal)
	}
	if matriculaFinal == nil || *matriculaFinal != "1234" {
		t.Errorf("matrícula = %v, esperava 1234", matriculaFinal)
	}
}

func TestFundirMembrosRecusaMesmoID(t *testing.T) {
	pool := testPool(t)
	repo := NewMembroRepository(pool)
	c := semearCenario(t, pool)

	if _, err := repo.FundirMembros(context.Background(), c.sobrevivente, c.sobrevivente, domain.MembroFusaoCampos{}); err == nil {
		t.Fatal("esperava erro ao fundir membro consigo mesmo")
	}
}

func TestGetFusaoPreviewSugereQuemTemMaisTarefas(t *testing.T) {
	pool := testPool(t)
	repo := NewMembroRepository(pool)
	ctx := context.Background()
	c := semearCenario(t, pool)

	// desempata: sobrevivente ganha uma terceira tarefa
	if _, err := pool.Exec(ctx, `
		INSERT INTO tarefas (fonte_dados_id, projeto_id, jira_id, numero_ticket, resumo, tipo, status, data_criacao, responsavel_id)
		VALUES ($1, $2, $3, $4, 'teste fusao', 'Bug', 'Concluído', NOW(), $5)
	`, c.fonte, c.projeto, uuid.NewString(), "TESTF-"+uuid.NewString()[:8], c.sobrevivente); err != nil {
		t.Fatalf("semeando tarefa extra: %v", err)
	}

	prev, err := repo.GetFusaoPreview(ctx, c.perdedor, c.sobrevivente)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if prev.SobreviventeSugeridoID != c.sobrevivente {
		t.Errorf("sugeriu %s, esperava %s", prev.SobreviventeSugeridoID, c.sobrevivente)
	}
	if prev.Membro.ID != c.perdedor || prev.Alvo.ID != c.sobrevivente {
		t.Errorf("lados trocados: membro=%s alvo=%s", prev.Membro.ID, prev.Alvo.ID)
	}
	if prev.Alvo.Tarefas != 3 || prev.Membro.Tarefas != 2 {
		t.Errorf("contagem de tarefas errada: membro=%d alvo=%d", prev.Membro.Tarefas, prev.Alvo.Tarefas)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run:
```bash
cd backend && MYPLANNER_TEST_DB_URL="postgres://myplanner:$(grep '^PASS_DB=' ../.env | cut -d= -f2)@localhost:5432/myplanner?sslmode=disable" go test ./internal/repository/ -run Fusao -v
```
Expected: FAIL na compilação, com `undefined: GetFusaoPreview` e `undefined: FundirMembros`.

- [ ] **Step 3: Implementar preview e fusão**

`backend/internal/repository/membro_fusao.go`:

```go
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetFusaoPreview traz os dois registros lado a lado para a tela de comparação.
// As contagens existem para o operador enxergar o tamanho da operação antes de
// confirmar — fusão não tem desfazer.
func (r *MembroRepository) GetFusaoPreview(ctx context.Context, aID, bID uuid.UUID) (*domain.MembroFusaoPreview, error) {
	if aID == bID {
		return nil, fmt.Errorf("não é possível fundir um membro com ele mesmo")
	}

	lados := map[uuid.UUID]domain.MembroFusaoLado{}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.nome, m.email, m.jira_account_id, m.cargo, m.salario, m.matricula,
		       m.data_admissao, m.ultimo_aumento, m.gestor_id, g.nome,
		       (SELECT count(*) FROM tarefas t WHERE t.responsavel_id = m.id),
		       (SELECT e.nome FROM equipe_membros em
		          JOIN equipes e ON e.id = em.equipe_id
		         WHERE em.membro_id = m.id AND em.data_saida IS NULL LIMIT 1),
		       (SELECT count(*) FROM historico_salario h WHERE h.membro_id = m.id),
		       (SELECT count(*) FROM membro_skills s WHERE s.membro_id = m.id),
		       (SELECT count(*) FROM disponibilidade d WHERE d.membro_id = m.id)
		FROM membros m
		LEFT JOIN membros g ON g.id = m.gestor_id
		WHERE m.id = ANY($1::uuid[])
	`, []uuid.UUID{aID, bID})
	if err != nil {
		return nil, fmt.Errorf("carregando membros da fusão: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var l domain.MembroFusaoLado
		if err := rows.Scan(&l.ID, &l.Nome, &l.Email, &l.JiraAccountID, &l.Cargo, &l.Salario,
			&l.Matricula, &l.DataAdmissao, &l.UltimoAumento, &l.GestorID, &l.GestorNome,
			&l.Tarefas, &l.EquipeNome, &l.RegistrosSalario, &l.Skills, &l.Ausencias); err != nil {
			return nil, fmt.Errorf("lendo membro da fusão: %w", err)
		}
		lados[l.ID] = l
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterando membros da fusão: %w", err)
	}

	a, okA := lados[aID]
	b, okB := lados[bID]
	if !okA || !okB {
		return nil, fmt.Errorf("membro não encontrado")
	}

	sugerido := a.ID
	if b.Tarefas > a.Tarefas {
		sugerido = b.ID
	}

	return &domain.MembroFusaoPreview{Membro: a, Alvo: b, SobreviventeSugeridoID: sugerido}, nil
}

// FundirMembros move tudo do perdedor para o sobrevivente e apaga o perdedor,
// numa transação só. A ordem importa: os filhos de membros são ON DELETE
// CASCADE e tarefas.responsavel_id é ON DELETE SET NULL, então apagar antes de
// repontar destruiria histórico.
func (r *MembroRepository) FundirMembros(ctx context.Context, sobreviventeID, perdedorID uuid.UUID, campos domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error) {
	if sobreviventeID == perdedorID {
		return nil, fmt.Errorf("não é possível fundir um membro com ele mesmo")
	}

	var dataAdmissao, ultimoAumento *time.Time
	if campos.DataAdmissao != nil {
		t, err := time.Parse("2006-01-02", *campos.DataAdmissao)
		if err != nil {
			return nil, fmt.Errorf("data_admissao inválida: %w", err)
		}
		dataAdmissao = &t
	}
	if campos.UltimoAumento != nil {
		t, err := time.Parse("2006-01-02", *campos.UltimoAumento)
		if err != nil {
			return nil, fmt.Errorf("ultimo_aumento inválido: %w", err)
		}
		ultimoAumento = &t
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// FOR UPDATE segura as duas linhas: sync rodando em paralelo não pode
	// atualizar o perdedor no meio da fusão.
	var perdedorFonte uuid.UUID
	var perdedorConta, perdedorNome string
	err = tx.QueryRow(ctx, `
		SELECT fonte_dados_id, jira_account_id, nome FROM membros WHERE id = $1 FOR UPDATE
	`, perdedorID).Scan(&perdedorFonte, &perdedorConta, &perdedorNome)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("membro perdedor não encontrado")
	}
	if err != nil {
		return nil, fmt.Errorf("carregando membro perdedor: %w", err)
	}

	var existeSobrevivente bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM membros WHERE id = $1 FOR UPDATE)`, sobreviventeID).Scan(&existeSobrevivente); err != nil {
		return nil, fmt.Errorf("carregando membro sobrevivente: %w", err)
	}
	if !existeSobrevivente {
		return nil, fmt.Errorf("membro sobrevivente não encontrado")
	}

	res := &domain.MembroFusaoResultado{}

	tag, err := tx.Exec(ctx, `UPDATE tarefas SET responsavel_id = $1 WHERE responsavel_id = $2`, sobreviventeID, perdedorID)
	if err != nil {
		return nil, fmt.Errorf("repontando responsável das tarefas: %w", err)
	}
	res.TarefasRepontadas = int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `UPDATE tarefas SET relator_id = $1 WHERE relator_id = $2`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando relator das tarefas: %w", err)
	}

	for _, tabela := range []string{"historico_salario", "membro_salarios", "membro_banco_horas", "disponibilidade"} {
		tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET membro_id = $1 WHERE membro_id = $2`, tabela), sobreviventeID, perdedorID)
		if err != nil {
			return nil, fmt.Errorf("repontando %s: %w", tabela, err)
		}
		res.RegistrosHistorico += int(tag.RowsAffected())
	}

	// UPDATE não aceita ON CONFLICT: move só o que o sobrevivente ainda não tem
	// e apaga o resto, senão o UNIQUE(membro_id, skill_id) estoura.
	if _, err := tx.Exec(ctx, `
		UPDATE membro_skills SET membro_id = $1
		 WHERE membro_id = $2
		   AND skill_id NOT IN (SELECT skill_id FROM membro_skills WHERE membro_id = $1)
	`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando skills: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM membro_skills WHERE membro_id = $1`, perdedorID); err != nil {
		return nil, fmt.Errorf("limpando skills duplicadas: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE membro_produtos SET membro_id = $1
		 WHERE membro_id = $2
		   AND produto_id NOT IN (SELECT produto_id FROM membro_produtos WHERE membro_id = $1)
	`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando produtos: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM membro_produtos WHERE membro_id = $1`, perdedorID); err != nil {
		return nil, fmt.Errorf("limpando produtos duplicados: %w", err)
	}

	// Vínculos encerrados podem ser repontados à vontade; o ativo esbarra em
	// UNIQUE(membro_id) WHERE data_saida IS NULL.
	if _, err := tx.Exec(ctx, `
		UPDATE equipe_membros SET membro_id = $1 WHERE membro_id = $2 AND data_saida IS NOT NULL
	`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando vínculos encerrados: %w", err)
	}

	var sobreviventeTemAtivo bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM equipe_membros WHERE membro_id = $1 AND data_saida IS NULL)
	`, sobreviventeID).Scan(&sobreviventeTemAtivo); err != nil {
		return nil, fmt.Errorf("verificando vínculo ativo do sobrevivente: %w", err)
	}
	if sobreviventeTemAtivo {
		tag, err := tx.Exec(ctx, `DELETE FROM equipe_membros WHERE membro_id = $1 AND data_saida IS NULL`, perdedorID)
		if err != nil {
			return nil, fmt.Errorf("encerrando vínculo duplicado: %w", err)
		}
		res.VinculoAtivoEncerrado = tag.RowsAffected() > 0
	} else if _, err := tx.Exec(ctx, `
		UPDATE equipe_membros SET membro_id = $1 WHERE membro_id = $2 AND data_saida IS NULL
	`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando vínculo ativo: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE membros SET gestor_id = $1 WHERE gestor_id = $2`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando gestor: %w", err)
	}
	// O sobrevivente pode ter acabado de virar gestor de si mesmo.
	if _, err := tx.Exec(ctx, `UPDATE membros SET gestor_id = NULL WHERE id = $1 AND gestor_id = $1`, sobreviventeID); err != nil {
		return nil, fmt.Errorf("limpando auto-referência de gestor: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE projetos SET lead_id = $1 WHERE lead_id = $2`, sobreviventeID, perdedorID); err != nil {
		return nil, fmt.Errorf("repontando lead de projetos: %w", err)
	}

	// Contas que o perdedor já tinha absorvido antes passam junto — é o que
	// dispensa regra especial para fusão em cadeia.
	tag, err = tx.Exec(ctx, `UPDATE membro_jira_contas SET membro_id = $1 WHERE membro_id = $2`, sobreviventeID, perdedorID)
	if err != nil {
		return nil, fmt.Errorf("movendo contas absorvidas: %w", err)
	}
	res.ContasAbsorvidas = int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `
		INSERT INTO membro_jira_contas (membro_id, fonte_dados_id, jira_account_id, nome_origem)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (fonte_dados_id, jira_account_id) DO UPDATE SET membro_id = EXCLUDED.membro_id
	`, sobreviventeID, perdedorFonte, perdedorConta, perdedorNome); err != nil {
		return nil, fmt.Errorf("registrando conta do perdedor: %w", err)
	}
	res.ContasAbsorvidas++

	// COALESCE: campo nil do operador significa "não alterar".
	if _, err := tx.Exec(ctx, `
		UPDATE membros SET
			cargo = COALESCE($2, cargo),
			salario = COALESCE($3, salario),
			matricula = COALESCE($4, matricula),
			data_admissao = COALESCE($5, data_admissao),
			ultimo_aumento = COALESCE($6, ultimo_aumento),
			gestor_id = COALESCE($7, gestor_id),
			updated_at = NOW()
		WHERE id = $1
	`, sobreviventeID, campos.Cargo, campos.Salario, campos.Matricula, dataAdmissao, ultimoAumento, campos.GestorID); err != nil {
		return nil, fmt.Errorf("gravando campos escolhidos: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM membros WHERE id = $1`, perdedorID); err != nil {
		return nil, fmt.Errorf("apagando membro perdedor: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit da fusão: %w", err)
	}
	return res, nil
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run:
```bash
cd backend && MYPLANNER_TEST_DB_URL="postgres://myplanner:$(grep '^PASS_DB=' ../.env | cut -d= -f2)@localhost:5432/myplanner?sslmode=disable" go test ./internal/repository/ -run Fusao -v
```
Expected: PASS nos quatro testes.

- [ ] **Step 5: Conferir que a suíte sem banco continua verde**

Run: `cd backend && go test ./...`
Expected: `ok` em todos os pacotes; os testes de fusão aparecem como skip.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/membro_fusao.go backend/internal/repository/membro_fusao_test.go
git commit -m "feat: repositório de fusão de membros com reponto transacional"
```

---

### Task 4: Sync resolve conta absorvida

**Files:**
- Modify: `backend/internal/repository/sync.go:63-77` (`UpsertMembro`)
- Modify: `backend/internal/repository/membro_fusao_test.go` (novo teste no fim do arquivo)

**Interfaces:**
- Consumes: tabela `membro_jira_contas` (Task 1), `FundirMembros` (Task 3).
- Produces: `UpsertMembro` mantém a assinatura `(ctx, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error)` e passa a devolver o ID do sobrevivente quando a conta foi absorvida.

- [ ] **Step 1: Escrever o teste que falha**

Acrescentar ao fim de `backend/internal/repository/membro_fusao_test.go`:

```go
func TestUpsertMembroDevolveSobreviventeParaContaAbsorvida(t *testing.T) {
	pool := testPool(t)
	membroRepo := NewMembroRepository(pool)
	syncRepo := NewSyncRepository(pool)
	ctx := context.Background()
	c := semearCenario(t, pool)

	var contaPerdedor, nomeSobrevivente string
	if err := pool.QueryRow(ctx, `SELECT jira_account_id FROM membros WHERE id = $1`, c.perdedor).Scan(&contaPerdedor); err != nil {
		t.Fatalf("lendo conta do perdedor: %v", err)
	}
	if _, err := membroRepo.FundirMembros(ctx, c.sobrevivente, c.perdedor, domain.MembroFusaoCampos{}); err != nil {
		t.Fatalf("fundindo: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT nome FROM membros WHERE id = $1`, c.sobrevivente).Scan(&nomeSobrevivente); err != nil {
		t.Fatalf("lendo nome do sobrevivente: %v", err)
	}

	// O sync veria a conta antiga com o displayName antigo.
	id, err := syncRepo.UpsertMembro(ctx, c.fonte, contaPerdedor, "Nome Antigo Do JIRA", nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert da conta absorvida: %v", err)
	}
	if id != c.sobrevivente {
		t.Errorf("UpsertMembro devolveu %s, esperava o sobrevivente %s", id, c.sobrevivente)
	}

	var nomeDepois string
	if err := pool.QueryRow(ctx, `SELECT nome FROM membros WHERE id = $1`, c.sobrevivente).Scan(&nomeDepois); err != nil {
		t.Fatalf("relendo nome: %v", err)
	}
	if nomeDepois != nomeSobrevivente {
		t.Errorf("nome do sobrevivente virou %q; a conta secundária não pode sobrescrever", nomeDepois)
	}

	var membrosComEssaConta int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM membros WHERE jira_account_id = $1`, contaPerdedor).Scan(&membrosComEssaConta); err != nil {
		t.Fatalf("contando membros recriados: %v", err)
	}
	if membrosComEssaConta != 0 {
		t.Errorf("sync recriou %d membro(s) para a conta absorvida", membrosComEssaConta)
	}
}
```

- [ ] **Step 2: Rodar o teste para ver falhar**

Run:
```bash
cd backend && MYPLANNER_TEST_DB_URL="postgres://myplanner:$(grep '^PASS_DB=' ../.env | cut -d= -f2)@localhost:5432/myplanner?sslmode=disable" go test ./internal/repository/ -run TestUpsertMembroDevolveSobrevivente -v
```
Expected: FAIL — `UpsertMembro` recria a pessoa e devolve um ID diferente do sobrevivente.

- [ ] **Step 3: Resolver alias antes do upsert**

Em `backend/internal/repository/sync.go`, substituir o corpo de `UpsertMembro` por:

```go
func (r *SyncRepository) UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error) {
	// Conta absorvida por fusão: devolve o sobrevivente e não toca no registro
	// — o nome dele não pode ser sobrescrito pelo displayName da conta antiga.
	var aliasID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT membro_id FROM membro_jira_contas
		 WHERE fonte_dados_id = $1 AND jira_account_id = $2
	`, fonteDadosID, jiraAccountID).Scan(&aliasID)
	if err == nil {
		return aliasID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("resolvendo conta absorvida %s: %w", jiraAccountID, err)
	}

	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO membros (id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (fonte_dados_id, jira_account_id)
		DO UPDATE SET nome = EXCLUDED.nome, email = EXCLUDED.email,
		              avatar_url = EXCLUDED.avatar_url, team = COALESCE(membros.team, EXCLUDED.team),
		              ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraAccountID, nome, email, avatarURL, team).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting membro %s: %w", jiraAccountID, err)
	}
	return id, nil
}
```

O arquivo já importa `github.com/jackc/pgx/v5`; falta `"errors"` — acrescentar ao bloco de import.

- [ ] **Step 4: Rodar os testes para ver passar**

Run:
```bash
cd backend && MYPLANNER_TEST_DB_URL="postgres://myplanner:$(grep '^PASS_DB=' ../.env | cut -d= -f2)@localhost:5432/myplanner?sslmode=disable" go test ./internal/repository/ -v
```
Expected: PASS, incluindo o teste novo.

- [ ] **Step 5: Conferir a suíte inteira**

Run: `cd backend && go build ./... && go test ./...`
Expected: build limpo e `ok` em todos os pacotes.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/sync.go backend/internal/repository/membro_fusao_test.go
git commit -m "feat: sync resolve conta JIRA absorvida para o membro sobrevivente"
```

---

### Task 5: Handler e rotas

**Files:**
- Create: `backend/internal/handler/membro_fusao.go`
- Create: `backend/internal/handler/membro_fusao_test.go`
- Modify: `backend/cmd/api/main.go` (perto de `membroHandler := handler.NewMembroHandler(...)`, linha ~107, e no bloco de rotas, linha ~291)

**Interfaces:**
- Consumes: `GetFusaoPreview` e `FundirMembros` (Task 3); tipos de `domain` (Task 2).
- Produces:
  - `GET /api/v1/membros/{id}/fusao-preview?alvo={uuid}` → `domain.MembroFusaoPreview`
  - `POST /api/v1/membros/{id}/fundir` (corpo `domain.MembroFusaoRequest`) → `domain.MembroFusaoResultado`; `{id}` é o sobrevivente. Consumidos pela Task 6.

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/handler/membro_fusao_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockMembroFusaoStore struct {
	previewFn func(ctx context.Context, aID, bID uuid.UUID) (*domain.MembroFusaoPreview, error)
	fundirFn  func(ctx context.Context, sobreviventeID, perdedorID uuid.UUID, campos domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error)
}

func (m *mockMembroFusaoStore) GetFusaoPreview(ctx context.Context, aID, bID uuid.UUID) (*domain.MembroFusaoPreview, error) {
	return m.previewFn(ctx, aID, bID)
}
func (m *mockMembroFusaoStore) FundirMembros(ctx context.Context, sobreviventeID, perdedorID uuid.UUID, campos domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error) {
	return m.fundirFn(ctx, sobreviventeID, perdedorID, campos)
}

func requestComID(metodo, alvo, corpo, id string) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestFusaoPreviewRepassaOsDoisIDs(t *testing.T) {
	membroID, alvoID := uuid.New(), uuid.New()
	var recebidoA, recebidoB uuid.UUID
	store := &mockMembroFusaoStore{
		previewFn: func(_ context.Context, a, b uuid.UUID) (*domain.MembroFusaoPreview, error) {
			recebidoA, recebidoB = a, b
			return &domain.MembroFusaoPreview{SobreviventeSugeridoID: b}, nil
		},
	}
	h := NewMembroFusaoHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Preview(w, requestComID(http.MethodGet, "/membros/x/fusao-preview?alvo="+alvoID.String(), "", membroID.String()))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	if recebidoA != membroID || recebidoB != alvoID {
		t.Errorf("IDs repassados errados: a=%s b=%s", recebidoA, recebidoB)
	}
}

func TestFusaoPreviewSemAlvoDa400(t *testing.T) {
	store := &mockMembroFusaoStore{previewFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.MembroFusaoPreview, error) {
		t.Fatal("não deveria chamar o store")
		return nil, nil
	}}
	h := NewMembroFusaoHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Preview(w, requestComID(http.MethodGet, "/membros/x/fusao-preview", "", uuid.NewString()))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperava 400", w.Code)
	}
}

func TestFundirRepassaSobreviventePerdedorECampos(t *testing.T) {
	sobrevivente, perdedor := uuid.New(), uuid.New()
	var gotSobrevivente, gotPerdedor uuid.UUID
	var gotCampos domain.MembroFusaoCampos
	store := &mockMembroFusaoStore{
		fundirFn: func(_ context.Context, s, p uuid.UUID, c domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error) {
			gotSobrevivente, gotPerdedor, gotCampos = s, p, c
			return &domain.MembroFusaoResultado{TarefasRepontadas: 231}, nil
		},
	}
	h := NewMembroFusaoHandler(store, zap.NewNop())

	corpo := `{"membro_perdedor_id":"` + perdedor.String() + `","campos":{"cargo":"analista_iii"}}`
	w := httptest.NewRecorder()
	h.Fundir(w, requestComID(http.MethodPost, "/membros/x/fundir", corpo, sobrevivente.String()))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	if gotSobrevivente != sobrevivente || gotPerdedor != perdedor {
		t.Errorf("IDs errados: sobrevivente=%s perdedor=%s", gotSobrevivente, gotPerdedor)
	}
	if gotCampos.Cargo == nil || *gotCampos.Cargo != "analista_iii" {
		t.Errorf("campos não repassados: %+v", gotCampos)
	}

	var res domain.MembroFusaoResultado
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	if res.TarefasRepontadas != 231 {
		t.Errorf("TarefasRepontadas = %d, esperava 231", res.TarefasRepontadas)
	}
}

func TestFundirComMesmoIDDa400(t *testing.T) {
	mesmo := uuid.New()
	store := &mockMembroFusaoStore{fundirFn: func(context.Context, uuid.UUID, uuid.UUID, domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error) {
		t.Fatal("não deveria chamar o store")
		return nil, nil
	}}
	h := NewMembroFusaoHandler(store, zap.NewNop())

	corpo := `{"membro_perdedor_id":"` + mesmo.String() + `"}`
	w := httptest.NewRecorder()
	h.Fundir(w, requestComID(http.MethodPost, "/membros/x/fundir", corpo, mesmo.String()))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperava 400", w.Code)
	}
}
```

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/handler/ -run Fus -v`
Expected: FAIL na compilação, com `undefined: NewMembroFusaoHandler`.

- [ ] **Step 3: Implementar o handler**

`backend/internal/handler/membro_fusao.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MembroFusaoStore interface {
	GetFusaoPreview(ctx context.Context, aID, bID uuid.UUID) (*domain.MembroFusaoPreview, error)
	FundirMembros(ctx context.Context, sobreviventeID, perdedorID uuid.UUID, campos domain.MembroFusaoCampos) (*domain.MembroFusaoResultado, error)
}

type MembroFusaoHandler struct {
	store  MembroFusaoStore
	logger *zap.Logger
}

func NewMembroFusaoHandler(store MembroFusaoStore, logger *zap.Logger) *MembroFusaoHandler {
	return &MembroFusaoHandler{store: store, logger: logger}
}

func (h *MembroFusaoHandler) Preview(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id de membro inválido")
		return
	}
	alvoID, err := uuid.Parse(r.URL.Query().Get("alvo"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "informe o parâmetro alvo com o id do outro membro")
		return
	}
	if membroID == alvoID {
		respondError(w, http.StatusBadRequest, "não é possível fundir um membro com ele mesmo")
		return
	}

	prev, err := h.store.GetFusaoPreview(r.Context(), membroID, alvoID)
	if err != nil {
		h.logger.Warn("failed to build fusao preview", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, prev)
}

// Fundir recebe o sobrevivente na URL e o perdedor no corpo. Não é idempotente:
// repetir sobre um par já fundido devolve 400, que é a resposta certa.
func (h *MembroFusaoHandler) Fundir(w http.ResponseWriter, r *http.Request) {
	sobreviventeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id de membro inválido")
		return
	}

	var req domain.MembroFusaoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.MembroPerdedorID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "informe membro_perdedor_id")
		return
	}
	if req.MembroPerdedorID == sobreviventeID {
		respondError(w, http.StatusBadRequest, "não é possível fundir um membro com ele mesmo")
		return
	}

	res, err := h.store.FundirMembros(r.Context(), sobreviventeID, req.MembroPerdedorID, req.Campos)
	if err != nil {
		h.logger.Error("failed to merge membros", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, res)
}
```

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/handler/ -run Fus -v`
Expected: PASS nos quatro testes.

- [ ] **Step 5: Ligar no main**

Em `backend/cmd/api/main.go`, logo depois de `membroHandler := handler.NewMembroHandler(membroRepo, logger)`:

```go
	membroFusaoHandler := handler.NewMembroFusaoHandler(membroRepo, logger)
```

E no bloco de rotas autenticadas, junto das outras rotas de `/membros/{id}` (depois de `r.Put("/membros/{id}/desligamento", membroHandler.UpdateDataDesligamento)`):

```go
			r.Get("/membros/{id}/fusao-preview", membroFusaoHandler.Preview)
			r.Post("/membros/{id}/fundir", membroFusaoHandler.Fundir)
```

- [ ] **Step 6: Subir e conferir a rota**

Run:
```bash
./dev.sh restart
set -a; source .env; set +a
TOKEN=$(curl -s -X POST localhost:9091/api/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin@myplanner.local\",\"senha\":\"$PASS_APP\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -s "localhost:9091/api/v1/membros/00000000-0000-0000-0000-000000000000/fusao-preview" -H "Authorization: Bearer $TOKEN"
```
Expected: `{"error":"informe o parâmetro alvo com o id do outro membro"}` — a rota existe e valida.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handler/membro_fusao.go backend/internal/handler/membro_fusao_test.go backend/cmd/api/main.go
git commit -m "feat: endpoints de preview e fusão de membros"
```

---

### Task 6: Interface de fusão na tela de Equipes

**Files:**
- Modify: `frontend/index.html` — CSS junto das regras de membro (perto de `.member-row`), markup do modal junto de `#merito-modal` (linha ~1597), botão no card de membro (linha ~2782), funções novas depois de `openTransferDropdown` (linha ~2568 em diante)

**Interfaces:**
- Consumes: `GET /membros/search?q=`, `GET /membros/{id}/fusao-preview?alvo=`, `POST /membros/{id}/fundir` (Task 5).
- Produces: nada consumido por outra task.

- [ ] **Step 1: Adicionar o markup do modal**

Depois do bloco `<div class="modal-overlay" id="merito-modal" …>…</div>` (linha ~1602):

```html
<div class="modal-overlay" id="fusao-modal" onclick="if(event.target===this)closeFusaoModal()">
  <div class="modal" style="max-width:720px">
    <div class="modal-title">⧉ Fundir membros duplicados</div>
    <div id="fusao-modal-body"></div>
  </div>
</div>
```

- [ ] **Step 2: Adicionar o CSS**

Junto das regras de membro no `<style>`:

```css
.fusao-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-bottom: 16px; }
.fusao-lado { border: 1px solid var(--border-subtle); border-radius: 10px; padding: 12px; }
.fusao-lado.fusao-sobrevivente { border-color: var(--accent); }
.fusao-lado-nome { font-size: 14px; font-weight: 600; color: var(--text-primary); margin-bottom: 4px; }
.fusao-lado-meta { font-size: 12px; color: var(--text-tertiary); line-height: 1.6; }
.fusao-campo { display: grid; grid-template-columns: 120px 1fr 1fr; gap: 8px; align-items: center; padding: 6px 0; border-top: 1px solid var(--border-subtle); font-size: 13px; }
.fusao-campo-nome { color: var(--text-tertiary); font-size: 12px; }
.fusao-campo-opcao { display: flex; align-items: center; gap: 6px; }
.fusao-aviso { background: rgba(220,38,38,.08); border: 1px solid rgba(220,38,38,.35); border-radius: 8px; padding: 10px 12px; font-size: 12px; color: var(--text-secondary); margin: 14px 0; line-height: 1.6; }
.fusao-busca-resultado { max-height: 220px; overflow-y: auto; }
```

- [ ] **Step 3: Adicionar o botão no card de membro**

Em `frontend/index.html`, no render de `.member-row` (linha ~2782), entre o botão de transferir e o de mérito:

```javascript
      '<button class="btn-member-action" onclick="event.stopPropagation();openFusaoBusca(\'' + m.id + '\')" title="Fundir com duplicado">⧉</button>' +
```

- [ ] **Step 4: Implementar as funções de fusão**

Depois de `openTransferDropdown`, acrescentar:

```javascript
// === FUSÃO DE MEMBROS DUPLICADOS ===
// A pessoa pode ter duas contas no JIRA e virar dois membros. Fundir apaga o
// perdedor e passa histórico e conta para o sobrevivente. Não tem desfazer.
var fusaoState = { membroId: null, alvoId: null, preview: null, sobreviventeId: null, escolhas: {} };

var FUSAO_CAMPOS = [
  { chave: 'cargo', rotulo: 'Cargo' },
  { chave: 'salario', rotulo: 'Salário' },
  { chave: 'matricula', rotulo: 'Matrícula' },
  { chave: 'data_admissao', rotulo: 'Admissão' },
  { chave: 'ultimo_aumento', rotulo: 'Último aumento' },
  { chave: 'gestor_id', rotulo: 'Gestor' }
];

function closeFusaoModal() {
  document.getElementById('fusao-modal').classList.remove('open');
}

function openFusaoBusca(membroId) {
  fusaoState = { membroId: membroId, alvoId: null, preview: null, sobreviventeId: null, escolhas: {} };
  document.getElementById('fusao-modal-body').innerHTML =
    '<div style="font-size:13px;color:var(--text-secondary);margin-bottom:10px">Busque o registro duplicado desta mesma pessoa:</div>' +
    '<input class="equipe-search" id="fusao-busca" placeholder="Nome do membro duplicado..." oninput="buscarMembroFusao()" />' +
    '<div class="fusao-busca-resultado" id="fusao-busca-resultado"></div>';
  document.getElementById('fusao-modal').classList.add('open');
  document.getElementById('fusao-busca').focus();
}

var _fusaoBuscaTimeout = null;
function buscarMembroFusao() {
  var q = document.getElementById('fusao-busca').value.trim();
  var alvo = document.getElementById('fusao-busca-resultado');
  if (q.length < 2) { alvo.innerHTML = ''; return; }
  clearTimeout(_fusaoBuscaTimeout);
  _fusaoBuscaTimeout = setTimeout(async function() {
    try {
      var membros = await api('/membros/search?q=' + encodeURIComponent(q));
      var lista = (membros || []).filter(function(m) { return m.id !== fusaoState.membroId; });
      if (lista.length === 0) {
        alvo.innerHTML = '<div style="padding:14px;font-size:13px;color:var(--text-tertiary);text-align:center">Nenhum membro encontrado</div>';
        return;
      }
      alvo.innerHTML = lista.map(function(m) {
        return '<div class="search-result-item" onclick="carregarFusaoPreview(\'' + m.id + '\')">' +
          '<div class="member-avatar" style="background:' + stringColor(m.nome) + ';width:30px;height:30px;font-size:11px">' + initials(m.nome) + '</div>' +
          '<div><div class="sr-name">' + esc(m.nome) + '</div><div class="sr-email">' + esc(m.email || '') + '</div></div></div>';
      }).join('');
    } catch (e) {
      alvo.innerHTML = '<div style="padding:14px;font-size:13px;color:var(--red)">' + esc(e.message) + '</div>';
    }
  }, 300);
}

async function carregarFusaoPreview(alvoId) {
  var body = document.getElementById('fusao-modal-body');
  body.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  try {
    var prev = await api('/membros/' + fusaoState.membroId + '/fusao-preview?alvo=' + alvoId);
    fusaoState.alvoId = alvoId;
    fusaoState.preview = prev;
    fusaoState.sobreviventeId = prev.sobrevivente_sugerido_id;
    fusaoState.escolhas = {};
    renderFusaoComparacao();
  } catch (e) {
    body.innerHTML = '<div class="fusao-aviso">' + esc(e.message) + '</div>' +
      '<div class="modal-actions"><button class="btn-cancel" type="button" onclick="closeFusaoModal()">Fechar</button></div>';
  }
}

function fusaoValorExibido(lado, chave) {
  var v = lado[chave];
  if (v === null || v === undefined || v === '') return null;
  if (chave === 'salario') return formatSalarioBR(v);
  if (chave === 'data_admissao' || chave === 'ultimo_aumento') return fmtDateBR(v);
  if (chave === 'cargo') return cargoSlugLabel(v);
  if (chave === 'gestor_id') return lado.gestor_nome || '—';
  return String(v);
}

function setFusaoSobrevivente(id) {
  fusaoState.sobreviventeId = id;
  renderFusaoComparacao();
}

function setFusaoEscolha(chave, ladoId) {
  fusaoState.escolhas[chave] = ladoId;
  renderFusaoComparacao();
}

function renderFusaoComparacao() {
  var p = fusaoState.preview;
  var lados = [p.membro, p.alvo];
  var sobrevivente = lados.filter(function(l) { return l.id === fusaoState.sobreviventeId; })[0];
  var perdedor = lados.filter(function(l) { return l.id !== fusaoState.sobreviventeId; })[0];

  var cabecalhos = lados.map(function(l) {
    var principal = l.id === fusaoState.sobreviventeId;
    return '<div class="fusao-lado' + (principal ? ' fusao-sobrevivente' : '') + '">' +
      '<div class="fusao-lado-nome">' + esc(l.nome) + '</div>' +
      '<div class="fusao-lado-meta">' + esc(l.email || 'sem e-mail') + '<br>' +
        'conta: ' + esc(l.jira_account_id) + '<br>' +
        l.tarefas + ' tarefas · ' + esc(l.equipe_nome || 'sem equipe') + '<br>' +
        l.registros_salario + ' registros de salário · ' + l.skills + ' skills · ' + l.ausencias + ' ausências</div>' +
      '<label class="fusao-campo-opcao" style="margin-top:8px">' +
        '<input type="radio" name="fusao-principal" ' + (principal ? 'checked' : '') +
        ' onchange="setFusaoSobrevivente(\'' + l.id + '\')"> Manter este' +
      '</label></div>';
  }).join('');

  var conflitosPendentes = 0;
  var camposHtml = FUSAO_CAMPOS.map(function(c) {
    var vS = fusaoValorExibido(sobrevivente, c.chave);
    var vP = fusaoValorExibido(perdedor, c.chave);
    if (vS === null && vP === null) return '';
    if (vS !== null && vP === null) {
      return '<div class="fusao-campo"><span class="fusao-campo-nome">' + esc(c.rotulo) + '</span>' +
        '<span>' + esc(vS) + '</span><span style="color:var(--text-tertiary)">—</span></div>';
    }
    if (vS === null && vP !== null) {
      // valor só existe no perdedor: adotado sem perguntar
      fusaoState.escolhas[c.chave] = perdedor.id;
      return '<div class="fusao-campo"><span class="fusao-campo-nome">' + esc(c.rotulo) + '</span>' +
        '<span style="color:var(--text-tertiary)">—</span><span>' + esc(vP) + ' <b>(adotado)</b></span></div>';
    }
    if (vS === vP) {
      return '<div class="fusao-campo"><span class="fusao-campo-nome">' + esc(c.rotulo) + '</span>' +
        '<span>' + esc(vS) + '</span><span>' + esc(vP) + '</span></div>';
    }
    var escolhido = fusaoState.escolhas[c.chave];
    if (!escolhido) conflitosPendentes++;
    return '<div class="fusao-campo"><span class="fusao-campo-nome">' + esc(c.rotulo) + '</span>' +
      '<label class="fusao-campo-opcao"><input type="radio" name="fusao-' + c.chave + '" ' +
        (escolhido === sobrevivente.id ? 'checked' : '') +
        ' onchange="setFusaoEscolha(\'' + c.chave + '\',\'' + sobrevivente.id + '\')">' + esc(vS) + '</label>' +
      '<label class="fusao-campo-opcao"><input type="radio" name="fusao-' + c.chave + '" ' +
        (escolhido === perdedor.id ? 'checked' : '') +
        ' onchange="setFusaoEscolha(\'' + c.chave + '\',\'' + perdedor.id + '\')">' + esc(vP) + '</label></div>';
  }).join('');

  var encerraVinculo = !!(sobrevivente.equipe_nome && perdedor.equipe_nome);
  var aviso = '<div class="fusao-aviso">' +
    '<b>' + sobrevivente.tarefas + ' + ' + perdedor.tarefas + ' tarefas</b> passam para ' + esc(sobrevivente.nome) + '. ' +
    (encerraVinculo ? '1 vínculo de equipe duplicado será encerrado. ' : '') +
    'O registro <b>' + esc(perdedor.nome) + '</b> será apagado e a conta JIRA dele passa a apontar para ' + esc(sobrevivente.nome) + '. ' +
    'Esta ação não tem desfazer.</div>';

  document.getElementById('fusao-modal-body').innerHTML =
    '<div class="fusao-grid">' + cabecalhos + '</div>' +
    (camposHtml ? '<div>' + camposHtml + '</div>' : '') +
    aviso +
    '<div class="modal-actions">' +
      '<button class="btn-cancel" type="button" onclick="closeFusaoModal()">Cancelar</button>' +
      '<button class="btn-remove-member" id="fusao-confirmar" type="button" ' + (conflitosPendentes > 0 ? 'disabled' : '') +
        ' onclick="confirmarFusao()">' +
        (conflitosPendentes > 0 ? 'Resolva ' + conflitosPendentes + ' conflito(s)' : 'Fundir') +
      '</button></div>';
}

async function confirmarFusao() {
  var p = fusaoState.preview;
  var lados = [p.membro, p.alvo];
  var sobrevivente = lados.filter(function(l) { return l.id === fusaoState.sobreviventeId; })[0];
  var perdedor = lados.filter(function(l) { return l.id !== fusaoState.sobreviventeId; })[0];

  var campos = {};
  FUSAO_CAMPOS.forEach(function(c) {
    var origemId = fusaoState.escolhas[c.chave];
    if (!origemId || origemId === sobrevivente.id) return; // nulo = não altera
    var origem = origemId === perdedor.id ? perdedor : sobrevivente;
    var v = origem[c.chave];
    if (v === null || v === undefined || v === '') return;
    if (c.chave === 'data_admissao' || c.chave === 'ultimo_aumento') v = String(v).slice(0, 10);
    campos[c.chave] = v;
  });

  var btn = document.getElementById('fusao-confirmar');
  btn.disabled = true;
  btn.textContent = 'Fundindo...';
  try {
    var res = await api('/membros/' + sobrevivente.id + '/fundir', {
      method: 'POST',
      body: JSON.stringify({ membro_perdedor_id: perdedor.id, campos: campos })
    });
    closeFusaoModal();
    alert(res.tarefas_repontadas + ' tarefa(s) movidas para ' + sobrevivente.nome + '. Registro duplicado apagado.');
    loadEquipeResumo();
  } catch (e) {
    btn.disabled = false;
    btn.textContent = 'Fundir';
    alert('Erro ao fundir: ' + e.message);
  }
}
```

- [ ] **Step 5: Conferir a sintaxe do JS**

Run:
```bash
cd /home/emerson/code/myplanner && python3 - <<'EOF'
import re
html = open('frontend/index.html').read()
blocos = re.findall(r'<script(?![^>]*src=)[^>]*>(.*?)</script>', html, re.S)
open('/tmp/app-check.js','w').write('\n;\n'.join(blocos))
EOF
node --check /tmp/app-check.js && echo "JS OK"
```
Expected: `JS OK`.

- [ ] **Step 6: Verificar no navegador**

O frontend é servido do disco (`http.ServeFile` em `main.go`), então basta recarregar.

1. Abrir `http://localhost:9091`, ir em Equipes e escolher uma equipe com membros.
2. Clicar em ⧉ num membro; buscar outro membro pelo nome.
3. Conferir na comparação: contagem de tarefas, equipe, e o botão bloqueado enquanto houver conflito de campo sem resposta.
4. **Cancelar sem confirmar** — a fusão de verdade é a Task 7.

Expected: modal abre, busca retorna, comparação monta, botão fica bloqueado com conflito pendente e liberado depois de escolher.

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat: tela de fusão de membros duplicados na página de Equipes"
```

---

### Task 7: Verificação no caso real (Paulo Cesar)

Esta task apaga um registro de produção do banco de dev e **não tem desfazer**. Só executar com o operador presente e ciente.

**Files:**
- Nenhum arquivo alterado.

**Interfaces:**
- Consumes: tudo das Tasks 1 a 6.
- Produces: confirmação de que o caso que originou a feature está resolvido.

- [ ] **Step 1: Backup do banco**

Run:
```bash
docker exec myplanner-db-1 pg_dump -U myplanner myplanner > /tmp/myplanner-antes-fusao.sql
ls -lh /tmp/myplanner-antes-fusao.sql
```
Expected: arquivo com tamanho maior que zero.

- [ ] **Step 2: Registrar o estado antes**

Run:
```bash
docker exec myplanner-db-1 psql -U myplanner -d myplanner -c "
SELECT m.id, m.nome, m.jira_account_id,
       (SELECT count(*) FROM tarefas t WHERE t.responsavel_id = m.id) AS tarefas,
       (SELECT e.nome FROM equipe_membros em JOIN equipes e ON e.id = em.equipe_id
         WHERE em.membro_id = m.id AND em.data_saida IS NULL) AS equipe
FROM membros m WHERE m.nome ILIKE 'paulo cesar w%' ORDER BY m.nome;"
```
Expected: duas linhas, ambas em Devops Varejo, com 231 e 67 tarefas.

- [ ] **Step 3: Fundir pela interface**

Na tela de Equipes → Devops Varejo, clicar ⧉ em `Paulo Cesar W`, buscar `Paulo Cesar Withoeft`, manter como principal o registro com mais tarefas, resolver os conflitos de campo (a matrícula e o cargo vieram da planilha para o Withoeft) e confirmar.

Expected: alerta informando as tarefas movidas; a equipe passa a listar uma pessoa.

- [ ] **Step 4: Conferir o resultado no banco**

Run:
```bash
docker exec myplanner-db-1 psql -U myplanner -d myplanner -c "
SELECT m.id, m.nome, m.jira_account_id,
       (SELECT count(*) FROM tarefas t WHERE t.responsavel_id = m.id) AS tarefas
FROM membros m WHERE m.nome ILIKE 'paulo cesar w%';
SELECT membro_id, jira_account_id, nome_origem FROM membro_jira_contas;"
```
Expected: uma linha em `membros` com 298 tarefas (231 + 67) e uma linha em `membro_jira_contas` com a conta do registro apagado e o `nome_origem` dele.

- [ ] **Step 5: Conferir que o sync não recria a duplicata**

Run: disparar o sync pela interface (ou aguardar o agendado) e repetir o comando do Step 4.
Expected: continua uma linha só em `membros` para a pessoa; nenhum membro novo com a conta absorvida.

- [ ] **Step 6: Conferir o relatório de esforço**

Abrir a página Esforço, filtrar pela equipe Devops Varejo e conferir que a pessoa aparece uma vez, com as horas somadas das duas contas.

Expected: uma linha por pessoa; total de cards da equipe inalterado.

---

## Notas de desvio da spec

- A spec previa testes de unidade em `service/membro_merge_test.go`. Não existe `MembroService` no repositório — `MembroHandler` conversa direto com `MembroRepository`. Os testes de unidade da validação ficam no handler (`handler/membro_fusao_test.go`, Task 5) e a lógica de banco é coberta pelos testes de integração (Tasks 3 e 4). Mesma cobertura, sem inventar uma camada.
- A spec escreveu `UPDATE … ON CONFLICT DO NOTHING` para `membro_skills` e `membro_produtos`. `UPDATE` não aceita `ON CONFLICT` no Postgres; a implementação usa `UPDATE … WHERE chave NOT IN (…)` seguido de `DELETE`, com o mesmo efeito.
