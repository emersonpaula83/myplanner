package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type TimelineRepository struct {
	pool *pgxpool.Pool
}

func NewTimelineRepository(pool *pgxpool.Pool) *TimelineRepository {
	return &TimelineRepository{pool: pool}
}

func (r *TimelineRepository) BuscarEpicosEquipe(ctx context.Context, equipeID uuid.UUID, ano int, projetoIDs []uuid.UUID) ([]domain.EpicoEquipe, error) {
	projetoFilter := ""
	args := []any{equipeID, ano}
	if len(projetoIDs) > 0 {
		projetoFilter = " AND e.projeto_id = ANY($3)"
		args = append(args, projetoIDs)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id, e.numero_ticket, e.resumo, e.status, e.apelido,
			e.data_inicio_execucao, e.data_limite, e.tipo_demanda,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c
				 WHERE c.parent_id = e.id
				   AND c.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $1)),
				0
			) AS total_segundos_equipe,
			EXISTS(
				SELECT 1 FROM tarefas p WHERE p.id = e.parent_id AND p.numero_ticket LIKE 'GDPTC-%'
			) AS projeto_ci,
			(SELECT p.numero_ticket FROM tarefas p WHERE p.id = e.parent_id AND p.numero_ticket LIKE 'GDPTC-%') AS projeto_ci_ticket
		FROM tarefas e
		WHERE e.tipo = 'Épico'
		  AND EXISTS (
		      SELECT 1 FROM tarefas ch
		      WHERE ch.parent_id = e.id
		        AND ch.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $1)
		  )
		  AND (
			  e.status IN ('Em Andamento', 'Desenvolvimento')
			  OR (e.status = 'Backlog' AND EXTRACT(YEAR FROM e.data_limite) = $2)
		  )
	`+projetoFilter+`
		ORDER BY
			CASE WHEN e.status IN ('Em Andamento', 'Desenvolvimento') THEN 0 ELSE 1 END,
			e.data_limite ASC NULLS LAST
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching epicos equipe: %w", err)
	}
	defer rows.Close()

	result := make([]domain.EpicoEquipe, 0)
	for rows.Next() {
		var e domain.EpicoEquipe
		if err := rows.Scan(
			&e.ID, &e.NumeroTicket, &e.Resumo, &e.Status, &e.Apelido,
			&e.DataInicioExecucao, &e.DataLimite, &e.TipoDemanda,
			&e.TotalSegundosEquipe,
			&e.ProjetoCI, &e.ProjetoCITicket,
		); err != nil {
			return nil, fmt.Errorf("scanning epico: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *TimelineRepository) ContarMembrosAtivosEquipe(ctx context.Context, equipeID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM equipe_membros em
		JOIN membros m ON m.id = em.membro_id
		WHERE em.equipe_id = $1 AND m.ativo = true
	`, equipeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting membros ativos: %w", err)
	}
	return count, nil
}

func (r *TimelineRepository) BuscarAusenciasMensais(ctx context.Context, equipeID uuid.UUID, ano int) ([]domain.AusenciaMensal, error) {
	inicioAno := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	fimAno := time.Date(ano, 12, 31, 0, 0, 0, 0, time.UTC)

	rows, err := r.pool.Query(ctx, `
		SELECT sub.membro_id, sub.nome, sub.tipo, sub.mes, COUNT(*)::int AS dias
		FROM (
			SELECT DISTINCT d.membro_id, m.nome, d.tipo,
			       EXTRACT(MONTH FROM dia)::int AS mes, dia::date
			FROM disponibilidade d
			JOIN membros m ON m.id = d.membro_id
			JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
			CROSS JOIN LATERAL generate_series(
				GREATEST(d.data_inicio, $2::date),
				LEAST(d.data_fim, $3::date),
				'1 day'::interval
			) dia
			WHERE m.ativo = true
			  AND d.tipo IN ('dayoff','ferias','licenca_medica','licenca_paternidade','licenca_maternidade')
			  AND d.data_fim >= $2::date
			  AND d.data_inicio <= $3::date
			  AND EXTRACT(DOW FROM dia) NOT IN (0, 6)
		) sub
		GROUP BY sub.membro_id, sub.nome, sub.tipo, sub.mes
		ORDER BY sub.mes, sub.nome
	`, equipeID, inicioAno, fimAno)
	if err != nil {
		return nil, fmt.Errorf("fetching ausencias mensais: %w", err)
	}
	defer rows.Close()

	result := make([]domain.AusenciaMensal, 0)
	for rows.Next() {
		var a domain.AusenciaMensal
		if err := rows.Scan(&a.MembroID, &a.Nome, &a.Tipo, &a.Mes, &a.Dias); err != nil {
			return nil, fmt.Errorf("scanning ausencia mensal: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (r *TimelineRepository) AtualizarMetadataProjeto(ctx context.Context, id uuid.UUID, apelido *string, dataInicioExecucao *time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET apelido = $2,
		    data_inicio_execucao = $3,
		    updated_at = NOW()
		WHERE id = $1 AND tipo = 'Épico'
	`, id, apelido, dataInicioExecucao)
	if err != nil {
		return fmt.Errorf("updating metadata projeto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("épico não encontrado")
	}
	return nil
}

func (r *TimelineRepository) BuscarEpicoPorID(ctx context.Context, id uuid.UUID) (*domain.Tarefa, error) {
	var t domain.Tarefa
	err := r.pool.QueryRow(ctx, `
		SELECT id, tipo, numero_ticket, resumo, apelido, data_inicio_execucao, data_limite
		FROM tarefas WHERE id = $1
	`, id).Scan(&t.ID, &t.Tipo, &t.NumeroTicket, &t.Resumo, &t.Apelido, &t.DataInicioExecucao, &t.DataLimite)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching epico by id: %w", err)
	}
	return &t, nil
}

func (r *TimelineRepository) ListarEpicos(ctx context.Context, equipeID *uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoListItem, error) {
	var rows pgx.Rows
	var err error

	projetoFilter := ""
	if equipeID != nil {
		args := []any{*equipeID}
		if len(projetoIDs) > 0 {
			projetoFilter = " AND e.projeto_id = ANY($2)"
			args = append(args, projetoIDs)
		}
		rows, err = r.pool.Query(ctx, `
			SELECT e.id, e.numero_ticket, e.resumo, e.apelido,
			       e.data_inicio_execucao, e.data_limite, e.tipo_demanda, e.status
			FROM tarefas e
			WHERE e.tipo = 'Épico'
			  AND EXISTS (
			      SELECT 1 FROM tarefas ch
			      WHERE ch.parent_id = e.id
			        AND ch.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $1)
			  )
		`+projetoFilter+`
			ORDER BY e.resumo
		`, args...)
	} else {
		if len(projetoIDs) > 0 {
			projetoFilter = " AND e.projeto_id = ANY($1)"
			rows, err = r.pool.Query(ctx, `
				SELECT e.id, e.numero_ticket, e.resumo, e.apelido,
				       e.data_inicio_execucao, e.data_limite, e.tipo_demanda, e.status
				FROM tarefas e
				WHERE e.tipo = 'Épico'
			`+projetoFilter+`
				ORDER BY e.resumo
			`, projetoIDs)
		} else {
			rows, err = r.pool.Query(ctx, `
				SELECT e.id, e.numero_ticket, e.resumo, e.apelido,
				       e.data_inicio_execucao, e.data_limite, e.tipo_demanda, e.status
				FROM tarefas e
				WHERE e.tipo = 'Épico'
				ORDER BY e.resumo
			`)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("listing epicos: %w", err)
	}
	defer rows.Close()

	result := make([]domain.ProjetoListItem, 0)
	for rows.Next() {
		var p domain.ProjetoListItem
		var dataLimite *time.Time
		if err := rows.Scan(
			&p.ID, &p.NumeroTicket, &p.Resumo, &p.Apelido,
			&p.DataInicioExecucao, &dataLimite, &p.TipoDemanda, &p.Status,
		); err != nil {
			return nil, fmt.Errorf("scanning epico: %w", err)
		}
		if dataLimite != nil {
			s := dataLimite.Format("2006-01-02")
			p.DataLimite = &s
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
