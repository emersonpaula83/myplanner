package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type EquipeRepository struct {
	pool *pgxpool.Pool
}

func NewEquipeRepository(pool *pgxpool.Pool) *EquipeRepository {
	return &EquipeRepository{pool: pool}
}

func (r *EquipeRepository) ListEquipes(ctx context.Context) ([]domain.Equipe, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome, created_at, updated_at FROM equipes ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Equipe, 0)
	for rows.Next() {
		var e domain.Equipe
		if err := rows.Scan(&e.ID, &e.Nome, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning equipe: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *EquipeRepository) GetEquipeByID(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
	var e domain.Equipe
	err := r.pool.QueryRow(ctx, `
		SELECT id, nome, created_at, updated_at FROM equipes WHERE id = $1
	`, id).Scan(&e.ID, &e.Nome, &e.CreatedAt, &e.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting equipe: %w", err)
	}
	return &e, nil
}

func (r *EquipeRepository) CreateEquipe(ctx context.Context, nome string) (*domain.Equipe, error) {
	var e domain.Equipe
	err := r.pool.QueryRow(ctx, `
		INSERT INTO equipes (nome) VALUES ($1)
		RETURNING id, nome, created_at, updated_at
	`, nome).Scan(&e.ID, &e.Nome, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating equipe: %w", err)
	}
	return &e, nil
}

func (r *EquipeRepository) UpdateEquipe(ctx context.Context, id uuid.UUID, nome string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE equipes SET nome = $2, updated_at = NOW() WHERE id = $1
	`, id, nome)
	if err != nil {
		return fmt.Errorf("updating equipe: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("equipe %s not found", id)
	}
	return nil
}

func (r *EquipeRepository) DeleteEquipe(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM equipes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting equipe: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("equipe %s not found", id)
	}
	return nil
}

func (r *EquipeRepository) GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
		       m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo, m.created_at, m.updated_at
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
		WHERE m.ativo = true
		  AND (m.data_desligamento IS NULL OR m.data_desligamento > NOW())
		ORDER BY m.nome
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting membros for equipe %s: %w", equipeID, err)
	}
	defer rows.Close()

	membros := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(
			&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
			&m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		membros = append(membros, m)
	}
	return membros, rows.Err()
}

func (r *EquipeRepository) AddMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO equipe_membros (equipe_id, membro_id) VALUES ($1, $2)
		ON CONFLICT (equipe_id, membro_id) DO NOTHING
	`, equipeID, membroID)
	if err != nil {
		return fmt.Errorf("adding membro to equipe: %w", err)
	}
	return nil
}

func (r *EquipeRepository) RemoveMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM equipe_membros WHERE equipe_id = $1 AND membro_id = $2
	`, equipeID, membroID)
	if err != nil {
		return fmt.Errorf("removing membro from equipe: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not in equipe %s", membroID, equipeID)
	}
	return nil
}

func (r *EquipeRepository) GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT membro_id, COUNT(*)::int AS dias_ausencia
		FROM (
		    SELECT DISTINCT membro_id, d::date AS dia
		    FROM disponibilidade,
		         generate_series(
		             GREATEST(data_inicio, $2::date),
		             LEAST(data_fim, $3::date),
		             '1 day'::interval
		         ) d
		    WHERE membro_id = ANY($1)
		      AND tipo IN ('dayoff','ferias','licenca_medica','licenca_paternidade','licenca_maternidade')
		      AND data_fim >= $2::date
		      AND data_inicio <= $3::date
		      AND EXTRACT(DOW FROM d) NOT IN (0, 6)
		) sub
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
		WITH t AS (
			SELECT *,
				COALESCE(tipo_demanda,
					CASE
						WHEN tipo IN ('Épico', 'Projeto') THEN 'Meta'
						WHEN tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
						ELSE 'Iniciativa'
					END
				) AS td
			FROM tarefas
			WHERE responsavel_id = ANY($1)
			  AND COALESCE(data_atualizado, data_criacao) >= $2
			  AND COALESCE(data_atualizado, data_criacao) < $3
		)
		SELECT
			responsavel_id,
			COALESCE(SUM(estimativa_tempo), 0) AS total_segundos,
			COALESCE(SUM(CASE WHEN td = 'Meta' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_metas,
			COALESCE(SUM(CASE WHEN td = 'Compromisso' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_compromissos,
			COALESCE(SUM(CASE WHEN td = 'Iniciativa' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_iniciativas,
			COALESCE(SUM(CASE WHEN td = 'Iniciativa' AND tipo IN ('Bug', 'Incidente', '[System] Incidente') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_manutencao,
			COALESCE(SUM(CASE WHEN td = 'Iniciativa' AND tipo IN ('Melhoria', 'História') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_melhorias,
			COALESCE(SUM(CASE WHEN td = 'Iniciativa' AND tipo IN ('Suporte', 'Support Needed', 'Tarefa', 'Subtarefa', 'Subtask') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_suporte,
			COALESCE(SUM(CASE WHEN status_categoria != 'done' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_estimado_abertos
		FROM t
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
			&h.SegundosManutencao, &h.SegundosMelhorias, &h.SegundosSuporte,
			&h.SegundosEstimadoAbertos,
		); err != nil {
			return nil, fmt.Errorf("scanning horas tarefas: %w", err)
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

func (r *EquipeRepository) UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET cargo = $2, updated_at = NOW() WHERE id = $1
	`, membroID, cargo)
	if err != nil {
		return fmt.Errorf("updating membro cargo: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", membroID)
	}
	return nil
}

func (r *EquipeRepository) ListProdutos(ctx context.Context) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_id, nome, descricao, projeto_id, ativo, created_at, updated_at
		FROM produtos
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing produtos: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Produto, 0)
	for rows.Next() {
		var p domain.Produto
		if err := rows.Scan(&p.ID, &p.FonteDadosID, &p.JiraID, &p.Nome, &p.Descricao, &p.ProjetoID, &p.Ativo, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning produto: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *EquipeRepository) GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.fonte_dados_id, p.jira_id, p.nome, p.descricao, p.projeto_id, p.ativo, p.created_at, p.updated_at
		FROM produtos p
		INNER JOIN membro_produtos mp ON mp.produto_id = p.id
		WHERE mp.membro_id = $1
		ORDER BY p.nome
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting membro produtos: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Produto, 0)
	for rows.Next() {
		var p domain.Produto
		if err := rows.Scan(&p.ID, &p.FonteDadosID, &p.JiraID, &p.Nome, &p.Descricao, &p.ProjetoID, &p.Ativo, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro produto: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *EquipeRepository) SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM membro_produtos WHERE membro_id = $1`, membroID); err != nil {
		return fmt.Errorf("clearing membro produtos: %w", err)
	}

	for _, pid := range produtoIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO membro_produtos (membro_id, produto_id) VALUES ($1, $2)
		`, membroID, pid); err != nil {
			return fmt.Errorf("inserting membro produto: %w", err)
		}
	}

	return tx.Commit(ctx)
}
