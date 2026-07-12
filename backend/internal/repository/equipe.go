package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type EquipeRepository struct {
	pool *pgxpool.Pool
}

func NewEquipeRepository(pool *pgxpool.Pool) *EquipeRepository {
	return &EquipeRepository{pool: pool}
}

func (r *EquipeRepository) ListEquipes(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT team
		FROM membros
		WHERE team IS NOT NULL AND ativo = true
		ORDER BY team
	`)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}
	defer rows.Close()

	teams := make([]string, 0)
	for rows.Next() {
		var team string
		if err := rows.Scan(&team); err != nil {
			return nil, fmt.Errorf("scanning team: %w", err)
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (r *EquipeRepository) GetMembrosEquipe(ctx context.Context, team string) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email,
		       avatar_url, team, ativo, created_at, updated_at
		FROM membros
		WHERE team = $1 AND ativo = true
		ORDER BY nome
	`, team)
	if err != nil {
		return nil, fmt.Errorf("getting membros for team %s: %w", team, err)
	}
	defer rows.Close()

	membros := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(
			&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
			&m.AvatarURL, &m.Team, &m.Ativo, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		membros = append(membros, m)
	}
	return membros, rows.Err()
}

func (r *EquipeRepository) GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT membro_id,
			COALESCE(SUM(
				(SELECT COUNT(*)
				 FROM generate_series(
					 GREATEST(data_inicio, $2::date),
					 LEAST(data_fim, $3::date),
					 '1 day'::interval
				 ) d
				 WHERE EXTRACT(DOW FROM d) NOT IN (0, 6))
			), 0)::int AS dias_ausencia
		FROM disponibilidade
		WHERE membro_id = ANY($1)
		  AND tipo IN ('dayoff', 'ferias', 'licenca_medica', 'licenca_paternidade', 'licenca_maternidade')
		  AND data_fim >= $2::date
		  AND data_inicio <= $3::date
		GROUP BY membro_id
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting dias ausencia: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]int)
	for rows.Next() {
		var membroID uuid.UUID
		var dias int
		if err := rows.Scan(&membroID, &dias); err != nil {
			return nil, fmt.Errorf("scanning dias ausencia: %w", err)
		}
		result[membroID] = dias
	}
	return result, rows.Err()
}

func (r *EquipeRepository) GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			responsavel_id,
			COALESCE(SUM(estimativa_tempo), 0) AS total_segundos,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Meta' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_metas,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Compromisso' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_compromissos,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_iniciativas,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo IN ('Bug', 'Incidente') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_manutencao,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo = 'Melhoria' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_melhorias,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo = 'História' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_evolucao,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo IN ('Suporte', 'Tarefa') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_suporte
		FROM tarefas
		WHERE responsavel_id = ANY($1)
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
		GROUP BY responsavel_id
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting horas tarefas equipe: %w", err)
	}
	defer rows.Close()

	result := make([]domain.HorasTarefasMembro, 0)
	for rows.Next() {
		var h domain.HorasTarefasMembro
		if err := rows.Scan(
			&h.MembroID, &h.TotalSegundos,
			&h.SegundosMetas, &h.SegundosCompromissos, &h.SegundosIniciativas,
			&h.SegundosManutencao, &h.SegundosMelhorias, &h.SegundosEvolucao, &h.SegundosSuporte,
		); err != nil {
			return nil, fmt.Errorf("scanning horas tarefas: %w", err)
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
