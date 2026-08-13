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
