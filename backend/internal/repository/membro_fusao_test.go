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
