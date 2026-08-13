package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type MembroRepository struct {
	pool *pgxpool.Pool
}

func NewMembroRepository(pool *pgxpool.Pool) *MembroRepository {
	return &MembroRepository{pool: pool}
}

func (r *MembroRepository) List(ctx context.Context) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *MembroRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
	var m domain.Membro
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros WHERE id = $1
	`, id).Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting membro: %w", err)
	}
	return &m, nil
}

func (r *MembroRepository) ListDisponibilidade(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, tipo, data_inicio, data_fim, descricao, criado_por, created_at, updated_at
		FROM disponibilidade
		WHERE membro_id = $1
		ORDER BY data_inicio DESC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("listing disponibilidade: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Disponibilidade, 0)
	for rows.Next() {
		var d domain.Disponibilidade
		if err := rows.Scan(&d.ID, &d.MembroID, &d.Tipo, &d.DataInicio, &d.DataFim, &d.Descricao, &d.CriadoPor, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning disponibilidade: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *MembroRepository) CreateDisponibilidade(ctx context.Context, d *domain.Disponibilidade) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO disponibilidade (id, membro_id, tipo, data_inicio, data_fim, descricao, criado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, d.ID, d.MembroID, d.Tipo, d.DataInicio, d.DataFim, d.Descricao, d.CriadoPor)
	if err != nil {
		return fmt.Errorf("creating disponibilidade: %w", err)
	}
	return nil
}

func (r *MembroRepository) UpdateDisponibilidade(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE disponibilidade SET tipo = $2, data_inicio = $3, data_fim = $4, descricao = $5, updated_at = NOW()
		WHERE id = $1
	`, id, tipo, dataInicio, dataFim, descricao)
	if err != nil {
		return fmt.Errorf("updating disponibilidade: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("disponibilidade %s not found", id)
	}
	return nil
}

func (r *MembroRepository) DeleteDisponibilidade(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM disponibilidade WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting disponibilidade: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("disponibilidade %s not found", id)
	}
	return nil
}

func (r *MembroRepository) GetMembroStats(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error) {
	var stats domain.MembroStats

	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TotalTarefas)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1 AND status_categoria = 'done'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TarefasConcluidas)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas concluidas: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1 AND status_categoria = 'indeterminate'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TarefasEmAndamento)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas em andamento: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			GREATEST(0,
				LEAST(data_fim, $3::date) - GREATEST(data_inicio, $2::date) + 1
			)
		), 0)
		FROM disponibilidade
		WHERE membro_id = $1 AND data_inicio <= $3 AND data_fim >= $2
	`, membroID, inicio, fim).Scan(&stats.DiasAusenteAno)
	if err != nil {
		return nil, fmt.Errorf("counting dias ausente: %w", err)
	}

	var segundosEstimados int64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(estimativa_tempo), 0) FROM tarefas
		WHERE responsavel_id = $1 AND status_categoria != 'done'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&segundosEstimados)
	if err != nil {
		return nil, fmt.Errorf("counting horas estimadas: %w", err)
	}
	stats.TotalHorasEstimadas = float64(segundosEstimados) / 3600.0

	return &stats, nil
}

func (r *MembroRepository) UpdateTeam(ctx context.Context, id uuid.UUID, team string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET team = $2, updated_at = NOW() WHERE id = $1
	`, id, team)
	if err != nil {
		return fmt.Errorf("updating membro team: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}

func (r *MembroRepository) Search(ctx context.Context, query string) ([]domain.Membro, error) {
	pattern := "%" + query + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros
		WHERE ativo = true AND (nome ILIKE $1 OR email ILIKE $1)
		ORDER BY nome
		LIMIT 50
	`, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *MembroRepository) UpdateDataDesligamento(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET data_desligamento = $2, updated_at = NOW() WHERE id = $1
	`, id, dataDesligamento)
	if err != nil {
		return fmt.Errorf("updating data_desligamento: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}

func (r *MembroRepository) ListTeams(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT team FROM membros WHERE team IS NOT NULL AND ativo = true ORDER BY team
	`)
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	defer rows.Close()

	teams := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scanning team: %w", err)
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

func (r *MembroRepository) UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE membros SET salario = $2, updated_at = NOW() WHERE id = $1
	`, id, valor)
	if err != nil {
		return fmt.Errorf("updating salario: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO membro_salarios (membro_id, valor, data_vigencia)
		VALUES ($1, $2, CURRENT_DATE)
	`, id, valor)
	if err != nil {
		return fmt.Errorf("inserting salary history: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *MembroRepository) UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE membros SET banco_horas = $2, updated_at = NOW() WHERE id = $1
	`, id, valor)
	if err != nil {
		return fmt.Errorf("updating banco_horas: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO membro_banco_horas (membro_id, valor)
		VALUES ($1, $2)
	`, id, valor)
	if err != nil {
		return fmt.Errorf("inserting banco_horas history: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *MembroRepository) UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET data_admissao = $2, updated_at = NOW() WHERE id = $1
	`, id, data)
	if err != nil {
		return fmt.Errorf("updating data_admissao: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}

func (r *MembroRepository) GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, valor, data_vigencia, created_at
		FROM membro_salarios
		WHERE membro_id = $1
		ORDER BY data_vigencia ASC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying salary history: %w", err)
	}
	defer rows.Close()

	var result []domain.SalarioHistorico
	for rows.Next() {
		var s domain.SalarioHistorico
		if err := rows.Scan(&s.ID, &s.MembroID, &s.Valor, &s.DataVigencia, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning salary history: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *MembroRepository) UpdateCamposImport(ctx context.Context, id uuid.UUID, salario *float64, cargo *string, dataAdmissao *time.Time, matricula *string, ultimoAumento *time.Time, gestorID *uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET
			salario = $2,
			cargo = COALESCE($3, cargo),
			data_admissao = $4,
			matricula = $5,
			ultimo_aumento = $6,
			gestor_id = $7,
			updated_at = NOW()
		WHERE id = $1
	`, id, salario, cargo, dataAdmissao, matricula, ultimoAumento, gestorID)
	if err != nil {
		return fmt.Errorf("updating campos import: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}

func (r *MembroRepository) InsertSalarioHistorico(ctx context.Context, membroID uuid.UUID, valor float64, dataVigencia time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO membro_salarios (membro_id, valor, data_vigencia)
		VALUES ($1, $2, $3)
	`, membroID, valor, dataVigencia)
	if err != nil {
		return fmt.Errorf("inserting salary history: %w", err)
	}
	return nil
}

func (r *MembroRepository) GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, valor, data_registro, created_at
		FROM membro_banco_horas
		WHERE membro_id = $1
		ORDER BY data_registro ASC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying banco_horas history: %w", err)
	}
	defer rows.Close()

	var result []domain.BancoHorasHistorico
	for rows.Next() {
		var b domain.BancoHorasHistorico
		if err := rows.Scan(&b.ID, &b.MembroID, &b.Valor, &b.DataRegistro, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning banco_horas history: %w", err)
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
