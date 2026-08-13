package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InvestimentoRepository struct {
	pool *pgxpool.Pool
}

func NewInvestimentoRepository(pool *pgxpool.Pool) *InvestimentoRepository {
	return &InvestimentoRepository{pool: pool}
}

// GetTopProdutosMembro returns the top N product names by task count for a member.
func (r *InvestimentoRepository) GetTopProdutosMembro(ctx context.Context, membroID uuid.UUID, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.nome, COUNT(*) as cnt
		FROM tarefas t
		JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
		JOIN produtos p ON p.id = tp.produto_id
		WHERE t.responsavel_id = $1
		  AND t.removido_em IS NULL
		GROUP BY p.nome
		ORDER BY cnt DESC
		LIMIT $2
	`, membroID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top produtos: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var nome string
		var cnt int
		if err := rows.Scan(&nome, &cnt); err != nil {
			return nil, fmt.Errorf("scanning top produto: %w", err)
		}
		result = append(result, nome)
	}
	return result, rows.Err()
}

// GetSalarioVigenteNoMes returns the salary in effect for a given month.
// It looks at membro_salarios for the most recent record with data_vigencia
// on or before the last day of the month. Returns nil when no salary
// history exists for the member before that month.
func (r *InvestimentoRepository) GetSalarioVigenteNoMes(ctx context.Context, membroID uuid.UUID, ano, mes int) (*float64, error) {
	lastDay := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC)

	var valor *float64
	err := r.pool.QueryRow(ctx, `
		SELECT valor FROM membro_salarios
		WHERE membro_id = $1 AND data_vigencia <= $2
		ORDER BY data_vigencia DESC, created_at DESC
		LIMIT 1
	`, membroID, lastDay).Scan(&valor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting salario vigente: %w", err)
	}
	return valor, nil
}

// GetMembrosEquipeNoMes returns members who were active in a given month
// (admitted on or before the end of the month, not terminated before the
// start of the month).
func (r *InvestimentoRepository) GetMembrosEquipeNoMes(ctx context.Context, equipeID uuid.UUID, ano, mes int) ([]domain.Membro, error) {
	firstDay := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC)

	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
		       m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo,
		       m.salario, m.data_admissao, m.banco_horas,
		       m.created_at, m.updated_at
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
		WHERE (m.data_admissao IS NULL OR m.data_admissao <= $2)
		  AND (m.data_desligamento IS NULL OR m.data_desligamento >= $3)
	`, equipeID, lastDay, firstDay)
	if err != nil {
		return nil, fmt.Errorf("querying membros for month: %w", err)
	}
	defer rows.Close()

	var membros []domain.Membro
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(
			&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
			&m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo,
			&m.Salario, &m.DataAdmissao, &m.BancoHoras,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		membros = append(membros, m)
	}
	return membros, rows.Err()
}

// GetAlocacoesProjetos returns project allocation percentages for a member,
// computed as hours on project / total hours * 100.
func (r *InvestimentoRepository) GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) ([]domain.ProjetoAlocacao, error) {
	rows, err := r.pool.Query(ctx, `
		WITH member_hours AS (
			SELECT
				p.nome AS apelido,
				p.jira_id AS chave_jira,
				COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0 AS horas
			FROM tarefas t
			JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
			JOIN produtos p ON p.id = tp.produto_id
			WHERE t.responsavel_id = $1
			  AND t.removido_em IS NULL
			  AND t.status NOT IN ('Cancelado', 'Rejeitada')
			GROUP BY p.nome, p.jira_id
		),
		total AS (
			SELECT COALESCE(SUM(horas), 0) AS total_horas FROM member_hours
		)
		SELECT
			mh.apelido,
			COALESCE(mh.chave_jira, ''),
			CASE WHEN t.total_horas > 0 THEN ROUND((mh.horas / t.total_horas * 100)::numeric, 1) ELSE 0 END
		FROM member_hours mh, total t
		ORDER BY mh.horas DESC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying project allocations: %w", err)
	}
	defer rows.Close()

	var result []domain.ProjetoAlocacao
	for rows.Next() {
		var p domain.ProjetoAlocacao
		if err := rows.Scan(&p.Apelido, &p.ChaveJira, &p.PercentualAlocacao); err != nil {
			return nil, fmt.Errorf("scanning allocation: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
