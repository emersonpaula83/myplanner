package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SprintRepository struct {
	pool *pgxpool.Pool
}

func NewSprintRepository(pool *pgxpool.Pool) *SprintRepository {
	return &SprintRepository{pool: pool}
}

func (r *SprintRepository) Pool() *pgxpool.Pool {
	return r.pool
}

type SprintListItem struct {
	ID           uuid.UUID  `json:"id"`
	Nome         string     `json:"nome"`
	Estado       *string    `json:"estado"`
	DataInicio   *time.Time `json:"data_inicio"`
	DataFim      *time.Time `json:"data_fim"`
	TotalTarefas int        `json:"total_tarefas"`
	ProjetoChave *string    `json:"projeto_chave,omitempty"`
	ProjetoNome  *string    `json:"projeto_nome,omitempty"`
	FonteDadosID *uuid.UUID `json:"fonte_dados_id,omitempty"`
}

type ProjetoComSprints struct {
	ID    uuid.UUID `json:"id"`
	Chave string    `json:"chave"`
	Nome  string    `json:"nome"`
}

func (r *SprintRepository) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]ProjetoComSprints, error) {
	var query string
	var args []interface{}

	if equipeID != nil {
		query = `
			SELECT DISTINCT p.id, p.chave, p.nome
			FROM projetos p
			INNER JOIN sprints s ON s.projeto_id = p.id
			INNER JOIN tarefas t ON t.projeto_id = p.id
			INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id
			WHERE p.ativo = true AND em.equipe_id = $1
			ORDER BY p.nome
		`
		args = []interface{}{*equipeID}
	} else {
		query = `
			SELECT DISTINCT p.id, p.chave, p.nome
			FROM projetos p
			INNER JOIN sprints s ON s.projeto_id = p.id
			WHERE p.ativo = true
			ORDER BY p.nome
		`
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing projetos com sprints: %w", err)
	}
	defer rows.Close()

	result := make([]ProjetoComSprints, 0)
	for rows.Next() {
		var p ProjetoComSprints
		if err := rows.Scan(&p.ID, &p.Chave, &p.Nome); err != nil {
			return nil, fmt.Errorf("scanning projeto: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}

func (r *SprintRepository) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]SprintListItem, error) {
	query := `
		SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim,
		       COUNT(t.id) AS total_tarefas
		FROM sprints s
		LEFT JOIN tarefas t ON t.sprint_id = s.id
		WHERE s.projeto_id = $1
	`
	args := []interface{}{projetoID}

	if estado != nil && *estado != "" {
		query += " AND s.estado = $2"
		args = append(args, *estado)
	}

	query += `
		GROUP BY s.id, s.nome, s.estado, s.data_inicio, s.data_fim
		ORDER BY s.data_inicio ASC NULLS LAST
	`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sprints by projeto: %w", err)
	}
	defer rows.Close()

	result := make([]SprintListItem, 0)
	for rows.Next() {
		var item SprintListItem
		if err := rows.Scan(&item.ID, &item.Nome, &item.Estado, &item.DataInicio, &item.DataFim, &item.TotalTarefas); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *SprintRepository) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]SprintListItem, error) {
	return r.listSprints(ctx, equipeID, estado, false)
}

func (r *SprintRepository) ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]SprintListItem, error) {
	return r.listSprints(ctx, equipeID, estado, true)
}

func (r *SprintRepository) listSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, includeEmpty bool) ([]SprintListItem, error) {
	query := `
		SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim,
		       (SELECT COUNT(*) FROM tarefas t WHERE t.sprint_id = s.id) AS total_tarefas,
		       p.chave, p.nome, s.fonte_dados_id
		FROM sprints s
		INNER JOIN projetos p ON p.id = s.projeto_id
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argN := 1

	if equipeID != nil {
		if includeEmpty {
			query += fmt.Sprintf(` AND (
				EXISTS (
					SELECT 1 FROM tarefas t2
					INNER JOIN equipe_membros em ON em.membro_id = t2.responsavel_id
					WHERE t2.sprint_id = s.id AND em.equipe_id = $%d
				)
				OR (
					NOT EXISTS (SELECT 1 FROM tarefas t3 WHERE t3.sprint_id = s.id)
					AND EXISTS (
						SELECT 1 FROM sprints s2
						INNER JOIN tarefas t4 ON t4.sprint_id = s2.id
						INNER JOIN equipe_membros em2 ON em2.membro_id = t4.responsavel_id
						WHERE s2.projeto_id = s.projeto_id AND em2.equipe_id = $%d
					)
				)
			)`, argN, argN)
		} else {
			query += fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM tarefas t2
				INNER JOIN equipe_membros em ON em.membro_id = t2.responsavel_id
				WHERE t2.sprint_id = s.id AND em.equipe_id = $%d
			)`, argN)
		}
		args = append(args, *equipeID)
		argN++
	}

	if estado != nil && *estado != "" {
		query += fmt.Sprintf(" AND s.estado = $%d", argN)
		args = append(args, *estado)
		argN++
	}

	if estado != nil && *estado == "closed" {
		query += " ORDER BY s.data_fim DESC NULLS LAST"
	} else {
		query += " ORDER BY s.data_inicio ASC NULLS LAST"
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sprints: %w", err)
	}
	defer rows.Close()

	result := make([]SprintListItem, 0)
	for rows.Next() {
		var item SprintListItem
		if err := rows.Scan(&item.ID, &item.Nome, &item.Estado, &item.DataInicio, &item.DataFim, &item.TotalTarefas, &item.ProjetoChave, &item.ProjetoNome, &item.FonteDadosID); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *SprintRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
	var s domain.Sprint
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, projeto_id, jira_id, nome, estado, data_inicio, data_fim, data_conclusao, board_id, created_at, updated_at
		FROM sprints WHERE id = $1
	`, id).Scan(&s.ID, &s.FonteDadosID, &s.ProjetoID, &s.JiraID, &s.Nome, &s.Estado, &s.DataInicio, &s.DataFim, &s.DataConclusao, &s.BoardID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting sprint %s: %w", id, err)
	}
	return &s, nil
}

type TarefaCapacity struct {
	ResponsavelID uuid.UUID
	Segundos      int64
}

func (r *SprintRepository) GetTarefasCapacityBySprint(ctx context.Context, sprintID uuid.UUID) ([]TarefaCapacity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT responsavel_id, COALESCE(estimativa_tempo, 0)
		FROM tarefas
		WHERE sprint_id = $1 AND responsavel_id IS NOT NULL
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas capacity: %w", err)
	}
	defer rows.Close()

	result := make([]TarefaCapacity, 0)
	for rows.Next() {
		var tc TarefaCapacity
		if err := rows.Scan(&tc.ResponsavelID, &tc.Segundos); err != nil {
			return nil, fmt.Errorf("scanning tarefa capacity: %w", err)
		}
		result = append(result, tc)
	}
	return result, nil
}

type TarefaDetail struct {
	ID             uuid.UUID `json:"id"`
	NumeroTicket   string    `json:"numero_ticket"`
	Resumo         string    `json:"resumo"`
	Tipo           string    `json:"tipo"`
	Status         string    `json:"status"`
	Prioridade     *string   `json:"prioridade"`
	Segundos       int64     `json:"estimativa_tempo"`
	ProjetoID      uuid.UUID `json:"projeto_id"`
	ProjetoChave   string    `json:"projeto_chave"`
	ProjetoNome    string    `json:"projeto_nome"`
	ResponsavelID  uuid.UUID `json:"-"`
}

func (r *SprintRepository) GetTarefasDetailBySprint(ctx context.Context, sprintID uuid.UUID) ([]TarefaDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       COALESCE(t.estimativa_tempo, 0),
		       p.id, p.chave, p.nome,
		       t.responsavel_id
		FROM tarefas t
		INNER JOIN projetos p ON p.id = t.projeto_id
		WHERE t.sprint_id = $1 AND t.responsavel_id IS NOT NULL
		ORDER BY p.chave, t.numero_ticket
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas detail: %w", err)
	}
	defer rows.Close()

	var result []TarefaDetail
	for rows.Next() {
		var td TarefaDetail
		if err := rows.Scan(&td.ID, &td.NumeroTicket, &td.Resumo, &td.Tipo, &td.Status,
			&td.Prioridade, &td.Segundos, &td.ProjetoID, &td.ProjetoChave, &td.ProjetoNome,
			&td.ResponsavelID); err != nil {
			return nil, fmt.Errorf("scanning tarefa detail: %w", err)
		}
		result = append(result, td)
	}
	return result, nil
}

type EqualizerTarefa struct {
	ID            uuid.UUID `json:"id"`
	NumeroTicket  string    `json:"numero_ticket"`
	Resumo        string    `json:"resumo"`
	Tipo          string    `json:"tipo"`
	Status        string    `json:"status"`
	Prioridade    *string   `json:"prioridade"`
	Horas         float64   `json:"horas"`
	ResponsavelID uuid.UUID `json:"-"`
}

// GetEqualizerTarefas returns tasks for a member in a sprint that have not
// yet started (i.e. not in an executed or in-progress status), ordered by
// hours descending so the equalizer's greedy algorithm can move the biggest
// tasks first. Status exclusion list mirrors GetCapacity's statusExecutado
// map plus the in-progress statuses.
func (r *SprintRepository) GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]EqualizerTarefa, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       COALESCE(t.estimativa_tempo, 0) / 3600.0
		FROM tarefas t
		WHERE t.sprint_id = $1
		  AND t.responsavel_id = $2
		  AND t.status NOT IN (
			'Code Review', 'Teste', 'Validação do Solicitante', 'Deploy', 'Concluído',
			'Em Desenvolvimento', 'Desenvolvimento', 'Cancelado'
		  )
		  AND COALESCE(t.estimativa_tempo, 0) > 0
		ORDER BY t.estimativa_tempo DESC
	`, sprintID, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting equalizer tarefas: %w", err)
	}
	defer rows.Close()

	var result []EqualizerTarefa
	for rows.Next() {
		var t EqualizerTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status, &t.Prioridade, &t.Horas); err != nil {
			return nil, fmt.Errorf("scanning equalizer tarefa: %w", err)
		}
		t.ResponsavelID = membroID
		result = append(result, t)
	}
	return result, nil
}

// UpdateTarefaResponsavel updates the local DB's assigned member for a task
// after the corresponding JIRA reassignment has been made.
func (r *SprintRepository) UpdateTarefaResponsavel(ctx context.Context, tarefaID, novoResponsavelID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET responsavel_id = $2, updated_at = NOW() WHERE id = $1
	`, tarefaID, novoResponsavelID)
	if err != nil {
		return fmt.Errorf("updating tarefa responsavel: %w", err)
	}
	return nil
}

// GetMembroJiraAccountID resolves the local membro UUID to its JIRA account
// ID, needed to call JIRA APIs such as AssignIssue.
func (r *SprintRepository) GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error) {
	var accountID string
	err := r.pool.QueryRow(ctx, `SELECT jira_account_id FROM membros WHERE id = $1`, membroID).Scan(&accountID)
	if err != nil {
		return "", fmt.Errorf("getting membro jira account id: %w", err)
	}
	return accountID, nil
}

type MembroInfo struct {
	ID               uuid.UUID
	Nome             string
	AvatarURL        *string
	DataDesligamento *time.Time
}

func (r *SprintRepository) GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]MembroInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT m.id, m.nome, m.avatar_url, m.data_desligamento
		FROM membros m
		INNER JOIN tarefas t ON t.responsavel_id = m.id
		WHERE t.sprint_id = $1
		ORDER BY m.nome
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting membros from sprint: %w", err)
	}
	defer rows.Close()

	result := make([]MembroInfo, 0)
	for rows.Next() {
		var m MembroInfo
		if err := rows.Scan(&m.ID, &m.Nome, &m.AvatarURL, &m.DataDesligamento); err != nil {
			return nil, fmt.Errorf("scanning membro info: %w", err)
		}
		result = append(result, m)
	}
	return result, nil
}

type AusenciaRecord struct {
	MembroID   uuid.UUID
	Tipo       string
	DataInicio time.Time
	DataFim    time.Time
}

func (r *SprintRepository) GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]AusenciaRecord, error) {
	if len(membroIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT membro_id, tipo, data_inicio, data_fim
		FROM disponibilidade
		WHERE membro_id = ANY($1)
		  AND data_inicio <= $3
		  AND data_fim >= $2
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting ausencias: %w", err)
	}
	defer rows.Close()

	result := make([]AusenciaRecord, 0)
	for rows.Next() {
		var a AusenciaRecord
		if err := rows.Scan(&a.MembroID, &a.Tipo, &a.DataInicio, &a.DataFim); err != nil {
			return nil, fmt.Errorf("scanning ausencia: %w", err)
		}
		result = append(result, a)
	}
	return result, nil
}

type FeriadoRecord struct {
	Data time.Time
	Nome string
}

func (r *SprintRepository) GetFeriadosNoPeriodo(ctx context.Context, inicio, fim time.Time) ([]FeriadoRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT data, nome FROM feriados
		WHERE data >= $1::date AND data <= $2::date
		ORDER BY data
	`, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting feriados: %w", err)
	}
	defer rows.Close()

	var result []FeriadoRecord
	for rows.Next() {
		var f FeriadoRecord
		if err := rows.Scan(&f.Data, &f.Nome); err != nil {
			return nil, fmt.Errorf("scanning feriado: %w", err)
		}
		result = append(result, f)
	}
	return result, nil
}

func (r *SprintRepository) GetMembrosEquipeIDs(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT em.membro_id FROM equipe_membros em
		JOIN membros m ON m.id = em.membro_id
		WHERE em.equipe_id = $1
		  AND (m.data_desligamento IS NULL OR m.data_desligamento > $2)
	`, equipeID, dataFim)
	if err != nil {
		return nil, fmt.Errorf("getting equipe membro ids: %w", err)
	}
	defer rows.Close()

	ids := make(map[uuid.UUID]bool)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning equipe membro id: %w", err)
		}
		ids[id] = true
	}
	return ids, nil
}

type UnplannedStats struct {
	TotalTarefas          int
	TarefasNaoPlanejadas  int
	HorasNaoPlanejadas    float64
	HorasTotalSprint      float64
	ManutencaoCount       int
	ManutencaoHoras       float64
	OutrasCount           int
	OutrasHoras           float64
}

func (r *SprintRepository) GetUnplannedStats(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*UnplannedStats, error) {
	var baseQuery string
	var args []interface{}

	naoPlanejadasFilter := `
		t.data_entrada_sprint > s.data_inicio
		OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
	`
	manutencaoFilter := `LOWER(t.tipo) IN ('bug') OR LOWER(t.tipo) LIKE '%incidente%'`

	if equipeID != nil {
		baseQuery = fmt.Sprintf(`
			SELECT
				COUNT(*) AS total_tarefas,
				COUNT(*) FILTER (WHERE %s) AS tarefas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE %s), 0) / 3600.0 AS horas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo), 0) / 3600.0 AS horas_total,
				COUNT(*) FILTER (WHERE (%s) AND (%s)) AS manutencao_count,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE (%s) AND (%s)), 0) / 3600.0 AS manutencao_horas,
				COUNT(*) FILTER (WHERE (%s) AND NOT (%s)) AS outras_count,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE (%s) AND NOT (%s)), 0) / 3600.0 AS outras_horas
			FROM tarefas t
			INNER JOIN sprints s ON s.id = t.sprint_id
			INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id
			WHERE t.sprint_id = $1 AND t.responsavel_id IS NOT NULL
			  AND s.data_inicio IS NOT NULL AND em.equipe_id = $2
		`, naoPlanejadasFilter, naoPlanejadasFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter)
		args = []interface{}{sprintID, *equipeID}
	} else {
		baseQuery = fmt.Sprintf(`
			SELECT
				COUNT(*) AS total_tarefas,
				COUNT(*) FILTER (WHERE %s) AS tarefas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE %s), 0) / 3600.0 AS horas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo), 0) / 3600.0 AS horas_total,
				COUNT(*) FILTER (WHERE (%s) AND (%s)) AS manutencao_count,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE (%s) AND (%s)), 0) / 3600.0 AS manutencao_horas,
				COUNT(*) FILTER (WHERE (%s) AND NOT (%s)) AS outras_count,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE (%s) AND NOT (%s)), 0) / 3600.0 AS outras_horas
			FROM tarefas t
			INNER JOIN sprints s ON s.id = t.sprint_id
			WHERE t.sprint_id = $1 AND t.responsavel_id IS NOT NULL
			  AND s.data_inicio IS NOT NULL
		`, naoPlanejadasFilter, naoPlanejadasFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter,
			naoPlanejadasFilter, manutencaoFilter)
		args = []interface{}{sprintID}
	}

	var stats UnplannedStats
	err := r.pool.QueryRow(ctx, baseQuery, args...).Scan(
		&stats.TotalTarefas,
		&stats.TarefasNaoPlanejadas,
		&stats.HorasNaoPlanejadas,
		&stats.HorasTotalSprint,
		&stats.ManutencaoCount,
		&stats.ManutencaoHoras,
		&stats.OutrasCount,
		&stats.OutrasHoras,
	)
	if err != nil {
		return nil, fmt.Errorf("getting unplanned stats: %w", err)
	}
	return &stats, nil
}

type HistoricalUnplannedItem struct {
	SprintID           uuid.UUID
	SprintNome         string
	HorasNaoPlanejadas float64
	HorasTotal         float64
	DiasUteis          int
	TotalMembros       int
}

func (r *SprintRepository) GetHistoricalUnplanned(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]HistoricalUnplannedItem, error) {
	var query string
	var args []interface{}

	if equipeID != nil {
		query = `
			SELECT s.id, s.nome,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE
					t.data_entrada_sprint > s.data_inicio
					OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
				), 0) / 3600.0 AS horas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo), 0) / 3600.0 AS horas_total,
				COUNT(DISTINCT t.responsavel_id) FILTER (WHERE em.equipe_id = $2) AS total_membros
			FROM sprints s
			INNER JOIN tarefas t ON t.sprint_id = s.id AND t.responsavel_id IS NOT NULL
			INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id AND em.equipe_id = $2
			WHERE s.projeto_id = $1 AND s.estado = 'closed'
			  AND s.data_inicio IS NOT NULL AND s.data_fim IS NOT NULL
			  AND s.id != $3
			GROUP BY s.id, s.nome, s.data_fim
			ORDER BY s.data_fim DESC
			LIMIT $4
		`
		args = []interface{}{projetoID, *equipeID, currentSprintID, limit}
	} else {
		query = `
			SELECT s.id, s.nome,
				COALESCE(SUM(t.estimativa_tempo) FILTER (WHERE
					t.data_entrada_sprint > s.data_inicio
					OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
				), 0) / 3600.0 AS horas_nao_planejadas,
				COALESCE(SUM(t.estimativa_tempo), 0) / 3600.0 AS horas_total,
				COUNT(DISTINCT t.responsavel_id) AS total_membros
			FROM sprints s
			INNER JOIN tarefas t ON t.sprint_id = s.id AND t.responsavel_id IS NOT NULL
			WHERE s.projeto_id = $1 AND s.estado = 'closed'
			  AND s.data_inicio IS NOT NULL AND s.data_fim IS NOT NULL
			  AND s.id != $2
			GROUP BY s.id, s.nome, s.data_fim
			ORDER BY s.data_fim DESC
			LIMIT $3
		`
		args = []interface{}{projetoID, currentSprintID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting historical unplanned: %w", err)
	}
	defer rows.Close()

	var result []HistoricalUnplannedItem
	for rows.Next() {
		var item HistoricalUnplannedItem
		if err := rows.Scan(&item.SprintID, &item.SprintNome, &item.HorasNaoPlanejadas, &item.HorasTotal, &item.TotalMembros); err != nil {
			return nil, fmt.Errorf("scanning historical unplanned: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *SprintRepository) GetEquipeNome(ctx context.Context, equipeID uuid.UUID) (string, error) {
	var nome string
	err := r.pool.QueryRow(ctx, `SELECT nome FROM equipes WHERE id = $1`, equipeID).Scan(&nome)
	if err != nil {
		return "", fmt.Errorf("getting equipe nome: %w", err)
	}
	return nome, nil
}

func (r *SprintRepository) GetSprintProjetoID(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error) {
	var projetoID *uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT projeto_id FROM sprints WHERE id = $1`, sprintID).Scan(&projetoID)
	if err != nil {
		return nil, fmt.Errorf("getting sprint projeto_id: %w", err)
	}
	return projetoID, nil
}

func (r *SprintRepository) GetProjetoChave(ctx context.Context, projetoID uuid.UUID) (string, error) {
	var chave string
	err := r.pool.QueryRow(ctx, `SELECT chave FROM projetos WHERE id = $1`, projetoID).Scan(&chave)
	if err != nil {
		return "", fmt.Errorf("getting projeto chave: %w", err)
	}
	return chave, nil
}

func (r *SprintRepository) GetMembrosEquipeInfo(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]MembroInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.nome, m.avatar_url, m.data_desligamento
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id
		WHERE em.equipe_id = $1
		  AND (m.data_desligamento IS NULL OR m.data_desligamento > $2)
		ORDER BY m.nome
	`, equipeID, dataFim)
	if err != nil {
		return nil, fmt.Errorf("getting equipe membros info: %w", err)
	}
	defer rows.Close()

	var result []MembroInfo
	for rows.Next() {
		var m MembroInfo
		if err := rows.Scan(&m.ID, &m.Nome, &m.AvatarURL, &m.DataDesligamento); err != nil {
			return nil, fmt.Errorf("scanning equipe membro info: %w", err)
		}
		result = append(result, m)
	}
	return result, nil
}

func (r *SprintRepository) GetAllMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]MembroInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.nome, m.avatar_url, m.data_desligamento
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id
		WHERE em.equipe_id = $1
		ORDER BY m.nome
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting all equipe membros: %w", err)
	}
	defer rows.Close()

	var result []MembroInfo
	for rows.Next() {
		var m MembroInfo
		if err := rows.Scan(&m.ID, &m.Nome, &m.AvatarURL, &m.DataDesligamento); err != nil {
			return nil, fmt.Errorf("scanning equipe membro: %w", err)
		}
		result = append(result, m)
	}
	return result, nil
}

type SprintHorasAlocadas struct {
	SprintID uuid.UUID
	Horas    float64
}

func (r *SprintRepository) GetHorasAlocadasPorSprint(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	if len(sprintIDs) == 0 || len(membroIDs) == 0 {
		return make(map[uuid.UUID]float64), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.sprint_id, COALESCE(SUM(t.estimativa_tempo), 0)
		FROM tarefas t
		WHERE t.sprint_id = ANY($1)
		  AND (t.responsavel_id = ANY($2) OR t.responsavel_id IS NULL)
		  AND t.status != 'Cancelado'
		GROUP BY t.sprint_id
	`, sprintIDs, membroIDs)
	if err != nil {
		return nil, fmt.Errorf("getting horas alocadas por sprint: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]float64)
	for rows.Next() {
		var sprintID uuid.UUID
		var segundos int64
		if err := rows.Scan(&sprintID, &segundos); err != nil {
			return nil, fmt.Errorf("scanning horas alocadas: %w", err)
		}
		result[sprintID] = float64(segundos) / 3600.0
	}
	return result, nil
}

type BurndownTarefa struct {
	EstimativaSegundos int
	DataResolvido      *time.Time
	DataEntradaSprint  *time.Time
	Status             string
}

func (r *SprintRepository) GetBurndownTarefas(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]BurndownTarefa, error) {
	query := `
		SELECT COALESCE(t.estimativa_tempo, 0), t.data_resolvido, t.data_entrada_sprint, t.status
		FROM tarefas t
		WHERE t.sprint_id = $1 AND t.status != 'Cancelado'
	`
	args := []any{sprintID}
	if equipeID != nil {
		query += ` AND t.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $2)`
		args = append(args, *equipeID)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting burndown tarefas: %w", err)
	}
	defer rows.Close()

	var result []BurndownTarefa
	for rows.Next() {
		var bt BurndownTarefa
		if err := rows.Scan(&bt.EstimativaSegundos, &bt.DataResolvido, &bt.DataEntradaSprint, &bt.Status); err != nil {
			return nil, fmt.Errorf("scanning burndown tarefa: %w", err)
		}
		result = append(result, bt)
	}
	return result, nil
}
