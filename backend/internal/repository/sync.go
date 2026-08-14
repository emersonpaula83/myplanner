package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SyncRepository struct {
	pool *pgxpool.Pool
}

func NewSyncRepository(pool *pgxpool.Pool) *SyncRepository {
	return &SyncRepository{pool: pool}
}

type UpsertTarefaParams struct {
	FonteDadosID       uuid.UUID
	ProjetoID          uuid.UUID
	JiraID             string
	NumeroTicket       string
	Resumo             string
	Tipo               string
	Status             string
	Prioridade         *string
	EstimativaPontos   *float64
	EstimativaTempo    *int
	TempoGasto         *int
	ResponsavelID      *uuid.UUID
	RelatorID          *uuid.UUID
	Team               *string
	SprintID           *uuid.UUID
	DataCriacao        time.Time
	DataLimite         *pgtype.Date
	DataResolvido      *time.Time
	DataAtualizado     *time.Time
	TipoDemanda        *string
	DataComponente     *pgtype.Date
	StatusCategoria    *string
	CamposExtras       json.RawMessage
	ParentID           *uuid.UUID
	Apelido            *string
	DataInicioExecucao *time.Time
	DataEntradaSprint  *time.Time
	Marcacao           bool
}

type SyncTotals struct {
	Projetos  int
	Tarefas   int
	Membros   int
	Sprints   int
	Removidos int
}

func (r *SyncRepository) UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO membros (id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (fonte_dados_id, jira_account_id)
		DO UPDATE SET nome = EXCLUDED.nome, email = EXCLUDED.email,
		              avatar_url = EXCLUDED.avatar_url, team = COALESCE(membros.team, EXCLUDED.team),
		              ativo = (membros.desativado_em IS NULL), updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraAccountID, nome, email, avatarURL, team).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting membro %s: %w", jiraAccountID, err)
	}
	return id, nil
}

func (r *SyncRepository) UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO projetos (id, fonte_dados_id, jira_id, chave, nome, descricao, lead_id, categoria, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, true)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET chave = EXCLUDED.chave, nome = EXCLUDED.nome,
		              descricao = EXCLUDED.descricao, lead_id = EXCLUDED.lead_id,
		              categoria = EXCLUDED.categoria, ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, chave, nome, descricao, leadID, categoria).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting projeto %s: %w", chave, err)
	}
	return id, nil
}

func (r *SyncRepository) UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error) {
	var currentEstado *string
	var existingID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id, estado FROM sprints WHERE fonte_dados_id = $1 AND jira_id = $2
	`, fonteDadosID, jiraID).Scan(&existingID, &currentEstado)

	isClosing := err == nil &&
		currentEstado != nil && *currentEstado == "active" &&
		estado != nil && *estado == "closed"

	if isClosing {
		r.captureSprintSnapshot(ctx, existingID)
	}

	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO sprints (id, fonte_dados_id, jira_id, nome, estado, data_inicio, data_fim, data_conclusao, board_id, projeto_id)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET nome = EXCLUDED.nome, estado = EXCLUDED.estado,
		              data_inicio = EXCLUDED.data_inicio, data_fim = EXCLUDED.data_fim,
		              data_conclusao = EXCLUDED.data_conclusao, board_id = EXCLUDED.board_id,
		              projeto_id = EXCLUDED.projeto_id, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, nome, estado, dataInicio, dataFim, dataConclusao, boardID, projetoID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting sprint %d: %w", jiraID, err)
	}
	return id, nil
}

type SnapshotTask struct {
	ID              uuid.UUID   `json:"id"`
	NumeroTicket    string      `json:"numero_ticket"`
	Resumo          string      `json:"resumo"`
	Tipo            string      `json:"tipo"`
	TipoDemanda     string      `json:"tipo_demanda"`
	Status          string      `json:"status"`
	ParentID        *uuid.UUID  `json:"parent_id"`
	RelatorNome     *string     `json:"relator_nome"`
	NaoPlanejada    bool        `json:"nao_planejada"`
	EstimativaTempo *int        `json:"estimativa_tempo"`
	Produtos        []string    `json:"produtos"`
	ProdutoIDs      []uuid.UUID `json:"produto_ids"`
}

type SnapshotMembroCapacity struct {
	MembroID        uuid.UUID `json:"membro_id"`
	Nome            string    `json:"nome"`
	HorasAlocadas   float64   `json:"horas_alocadas"`
	HorasExecutadas float64   `json:"horas_executadas"`
	HorasDisponiveis float64  `json:"horas_disponiveis"`
}

type SnapshotCapacity struct {
	DiasUteis              int                      `json:"dias_uteis"`
	HorasTotalSprint       float64                  `json:"horas_total_sprint"`
	HorasAlocadas          float64                  `json:"horas_alocadas"`
	HorasExecutadas        float64                  `json:"horas_executadas"`
	HorasPendentesExecucao float64                  `json:"horas_pendentes_execucao"`
	TotalMembros           int                      `json:"total_membros"`
	Membros                []SnapshotMembroCapacity  `json:"membros"`
}

type SnapshotBurndownPoint struct {
	Data  string  `json:"data"`
	Horas float64 `json:"horas"`
}

type SnapshotBurndown struct {
	HorasTotal     float64                 `json:"horas_total"`
	LinhaIdeal     []SnapshotBurndownPoint `json:"linha_ideal"`
	LinhaReal      []SnapshotBurndownPoint `json:"linha_real"`
	LinhaUnplanned []SnapshotBurndownPoint `json:"linha_nao_planejadas"`
}

type SprintSnapshotV2 struct {
	Version    int                `json:"version"`
	Tarefas    []SnapshotTask     `json:"tarefas"`
	Capacidade *SnapshotCapacity  `json:"capacidade,omitempty"`
	Burndown   *SnapshotBurndown  `json:"burndown,omitempty"`
}

func (r *SyncRepository) captureSprintSnapshot(ctx context.Context, sprintID uuid.UUID) {
	tasks := r.captureSnapshotTasks(ctx, sprintID)
	capacity := r.captureSnapshotCapacity(ctx, sprintID)
	burndown := r.captureSnapshotBurndown(ctx, sprintID)

	snapshot := SprintSnapshotV2{
		Version:    2,
		Tarefas:    tasks,
		Capacidade: capacity,
		Burndown:   burndown,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}

	r.pool.Exec(ctx, `
		INSERT INTO sprint_review_snapshots (sprint_id, snapshot_json)
		VALUES ($1, $2)
		ON CONFLICT (sprint_id) DO NOTHING
	`, sprintID, data)
}

func (r *SyncRepository) captureSnapshotTasks(ctx context.Context, sprintID uuid.UUID) []SnapshotTask {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo,
		       COALESCE(t.tipo_demanda,
		           CASE
		               WHEN t.tipo IN ('Épico', 'Projeto') THEN 'Meta'
		               WHEN t.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
		               ELSE 'Iniciativa'
		           END
		       ),
		       t.status,
		       t.parent_id, m.nome,
		       CASE WHEN t.data_entrada_sprint > s.data_inicio
		            OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
		            THEN true ELSE false END AS nao_planejada,
		       t.estimativa_tempo,
		       ARRAY_AGG(p.nome ORDER BY p.id) FILTER (WHERE p.nome IS NOT NULL) AS produtos,
		       ARRAY_AGG(p.id ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL) AS produto_ids
		FROM tarefas t
		INNER JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.relator_id
		LEFT JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
		LEFT JOIN produtos p ON p.id = tp.produto_id
		WHERE t.sprint_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		GROUP BY t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
		         t.parent_id, m.nome, t.data_entrada_sprint, t.data_criacao,
		         s.data_inicio, t.estimativa_tempo
		ORDER BY t.numero_ticket
	`, sprintID)
	if err != nil {
		return []SnapshotTask{}
	}
	defer rows.Close()

	tasks := make([]SnapshotTask, 0)
	for rows.Next() {
		var t SnapshotTask
		if err := rows.Scan(
			&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda,
			&t.Status, &t.ParentID, &t.RelatorNome, &t.NaoPlanejada,
			&t.EstimativaTempo, &t.Produtos, &t.ProdutoIDs,
		); err != nil {
			return tasks
		}
		tasks = append(tasks, t)
	}
	return tasks
}

func (r *SyncRepository) captureSnapshotCapacity(ctx context.Context, sprintID uuid.UUID) *SnapshotCapacity {
	var dataInicio, dataFim *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT data_inicio, data_fim FROM sprints WHERE id = $1
	`, sprintID).Scan(&dataInicio, &dataFim)
	if err != nil || dataInicio == nil || dataFim == nil {
		return nil
	}

	var feriadoCount int
	r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM feriados
		WHERE data BETWEEN $1 AND $2
		  AND EXTRACT(DOW FROM data) NOT IN (0, 6)
	`, dataInicio, dataFim).Scan(&feriadoCount)

	diasUteis := 0
	for d := *dataInicio; !d.After(*dataFim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			diasUteis++
		}
	}
	diasUteis -= feriadoCount

	statusExecutado := map[string]bool{
		"Code Review": true, "Teste": true, "Validação do Solicitante": true, "Deploy": true, "Concluído": true,
	}
	statusPendente := map[string]bool{
		"Backlog": true, "Desenvolvimento": true, "Em Desenvolvimento": true, "A Fazer": true,
	}

	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(t.responsavel_id, '00000000-0000-0000-0000-000000000000'),
		       COALESCE(m.nome, 'Sem responsável'),
		       t.status,
		       COALESCE(t.estimativa_tempo, 0)
		FROM tarefas t
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.sprint_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
	`, sprintID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type membroAcc struct {
		nome      string
		alocadas  float64
		executadas float64
	}
	membrosMap := make(map[uuid.UUID]*membroAcc)

	var totalAlocadas, totalExecutadas, totalPendentes float64

	for rows.Next() {
		var membroID uuid.UUID
		var nome, status string
		var segundos int
		if err := rows.Scan(&membroID, &nome, &status, &segundos); err != nil {
			continue
		}
		horas := float64(segundos) / 3600.0

		if _, ok := membrosMap[membroID]; !ok {
			membrosMap[membroID] = &membroAcc{nome: nome}
		}
		acc := membrosMap[membroID]

		if statusExecutado[status] {
			acc.executadas += horas
			totalExecutadas += horas
		} else {
			acc.alocadas += horas
			totalAlocadas += horas
			if statusPendente[status] {
				totalPendentes += horas
			}
		}
	}

	horasPorDia := 6.0
	horasTotalSprint := float64(diasUteis) * horasPorDia * float64(len(membrosMap))

	membros := make([]SnapshotMembroCapacity, 0, len(membrosMap))
	for id, acc := range membrosMap {
		membros = append(membros, SnapshotMembroCapacity{
			MembroID:         id,
			Nome:             acc.nome,
			HorasAlocadas:    math.Round(acc.alocadas*10) / 10,
			HorasExecutadas:  math.Round(acc.executadas*10) / 10,
			HorasDisponiveis: math.Round(float64(diasUteis)*horasPorDia*10) / 10,
		})
	}

	return &SnapshotCapacity{
		DiasUteis:              diasUteis,
		HorasTotalSprint:       math.Round(horasTotalSprint*10) / 10,
		HorasAlocadas:          math.Round(totalAlocadas*10) / 10,
		HorasExecutadas:        math.Round(totalExecutadas*10) / 10,
		HorasPendentesExecucao: math.Round(totalPendentes*10) / 10,
		TotalMembros:           len(membrosMap),
		Membros:                membros,
	}
}

func (r *SyncRepository) captureSnapshotBurndown(ctx context.Context, sprintID uuid.UUID) *SnapshotBurndown {
	var dataInicio, dataFim *time.Time
	var sprintNome string
	err := r.pool.QueryRow(ctx, `
		SELECT nome, data_inicio, data_fim FROM sprints WHERE id = $1
	`, sprintID).Scan(&sprintNome, &dataInicio, &dataFim)
	if err != nil || dataInicio == nil || dataFim == nil {
		return nil
	}

	var feriadoDates []time.Time
	fRows, err := r.pool.Query(ctx, `
		SELECT data FROM feriados WHERE data BETWEEN $1 AND $2
	`, dataInicio, dataFim)
	if err == nil {
		defer fRows.Close()
		for fRows.Next() {
			var d time.Time
			if fRows.Scan(&d) == nil {
				if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
					feriadoDates = append(feriadoDates, d)
				}
			}
		}
	}
	feriadoSet := make(map[string]bool)
	for _, d := range feriadoDates {
		feriadoSet[d.Format("2006-01-02")] = true
	}

	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(t.estimativa_tempo, 0), t.data_resolvido, t.data_entrada_sprint, t.status
		FROM tarefas t
		WHERE t.sprint_id = $1 AND t.status NOT IN ('Cancelado', 'Rejeitada')
	`, sprintID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type burndownTask struct {
		estimativa        int
		dataResolvido     *time.Time
		dataEntradaSprint *time.Time
		status            string
	}
	var tarefas []burndownTask
	for rows.Next() {
		var bt burndownTask
		if err := rows.Scan(&bt.estimativa, &bt.dataResolvido, &bt.dataEntradaSprint, &bt.status); err != nil {
			continue
		}
		tarefas = append(tarefas, bt)
	}

	var diasUteis []string
	for d := *dataInicio; !d.After(*dataFim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday && !feriadoSet[d.Format("2006-01-02")] {
			diasUteis = append(diasUteis, d.Format("2006-01-02"))
		}
	}
	if len(diasUteis) == 0 {
		return nil
	}

	var horasIniciais float64
	for _, t := range tarefas {
		if t.dataEntradaSprint != nil && !t.dataEntradaSprint.After(*dataInicio) {
			horasIniciais += float64(t.estimativa) / 3600.0
		}
	}

	linhaIdeal := make([]SnapshotBurndownPoint, len(diasUteis))
	decremento := horasIniciais / float64(len(diasUteis))
	for i, d := range diasUteis {
		linhaIdeal[i] = SnapshotBurndownPoint{
			Data:  d,
			Horas: math.Round((horasIniciais-decremento*float64(i))*10) / 10,
		}
	}

	statusDesconto80 := map[string]bool{
		"Teste": true, "Validação do Solicitante": true, "Deploy": true,
	}

	linhaReal := make([]SnapshotBurndownPoint, len(diasUteis))
	horasRestantes := horasIniciais
	now := time.Now()
	for i, d := range diasUteis {
		dDate, _ := time.Parse("2006-01-02", d)
		if dDate.After(now) {
			linhaReal = linhaReal[:i]
			break
		}
		for _, t := range tarefas {
			if t.dataEntradaSprint != nil && !t.dataEntradaSprint.After(*dataInicio) {
				if t.dataResolvido != nil && t.dataResolvido.Format("2006-01-02") == d {
					horasRestantes -= float64(t.estimativa) / 3600.0
				}
			}
		}
		horas := horasRestantes
		for _, t := range tarefas {
			if t.dataResolvido == nil && statusDesconto80[t.status] {
				if t.dataEntradaSprint != nil && !t.dataEntradaSprint.After(*dataInicio) {
					horas -= float64(t.estimativa) / 3600.0 * 0.8
				}
			}
		}
		linhaReal[i] = SnapshotBurndownPoint{
			Data:  d,
			Horas: math.Round(horas*10) / 10,
		}
	}

	linhaUnplanned := make([]SnapshotBurndownPoint, len(diasUteis))
	horasNaoPlanejadas := 0.0
	for i, d := range diasUteis {
		for _, t := range tarefas {
			if t.dataEntradaSprint != nil {
				entradaStr := t.dataEntradaSprint.Format("2006-01-02")
				if entradaStr == d && t.dataEntradaSprint.After(*dataInicio) {
					horasNaoPlanejadas += float64(t.estimativa) / 3600.0
				}
			}
		}
		linhaUnplanned[i] = SnapshotBurndownPoint{
			Data:  d,
			Horas: math.Round(horasNaoPlanejadas*10) / 10,
		}
	}

	return &SnapshotBurndown{
		HorasTotal:     math.Round(horasIniciais*10) / 10,
		LinhaIdeal:     linhaIdeal,
		LinhaReal:      linhaReal,
		LinhaUnplanned: linhaUnplanned,
	}
}

func (r *SyncRepository) GetDistinctBoardProjects(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT board_id, projeto_id
		FROM sprints
		WHERE fonte_dados_id = $1 AND board_id IS NOT NULL AND projeto_id IS NOT NULL
	`, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting board projects: %w", err)
	}
	defer rows.Close()
	result := make(map[int]uuid.UUID)
	for rows.Next() {
		var boardID int
		var projetoID uuid.UUID
		if err := rows.Scan(&boardID, &projetoID); err != nil {
			return nil, err
		}
		result[boardID] = projetoID
	}
	return result, nil
}

func (r *SyncRepository) UpsertTarefa(ctx context.Context, t *UpsertTarefaParams) (uuid.UUID, error) {
	ce := t.CamposExtras
	if ce == nil {
		ce = json.RawMessage(`{}`)
	}
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tarefas (id, fonte_dados_id, projeto_id, jira_id, numero_ticket, resumo,
		                     tipo, status, prioridade, estimativa_pontos, estimativa_tempo, tempo_gasto,
		                     responsavel_id, relator_id, team, sprint_id, data_criacao, data_limite,
		                     data_resolvido, data_atualizado, tipo_demanda, data_componente,
		                     status_categoria, campos_extras, parent_id, apelido, data_inicio_execucao,
		                     data_entrada_sprint, marcacao)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET resumo = EXCLUDED.resumo, tipo = EXCLUDED.tipo, status = EXCLUDED.status,
		              prioridade = EXCLUDED.prioridade, estimativa_pontos = EXCLUDED.estimativa_pontos,
		              estimativa_tempo = EXCLUDED.estimativa_tempo, tempo_gasto = EXCLUDED.tempo_gasto,
		              responsavel_id = EXCLUDED.responsavel_id, relator_id = EXCLUDED.relator_id,
		              team = EXCLUDED.team, sprint_id = EXCLUDED.sprint_id,
		              data_limite = COALESCE(EXCLUDED.data_limite, tarefas.data_limite),
		              data_resolvido = EXCLUDED.data_resolvido,
		              data_atualizado = EXCLUDED.data_atualizado, tipo_demanda = EXCLUDED.tipo_demanda,
		              data_componente = EXCLUDED.data_componente, status_categoria = EXCLUDED.status_categoria,
		              parent_id = EXCLUDED.parent_id,
		              apelido = COALESCE(EXCLUDED.apelido, tarefas.apelido),
		              data_inicio_execucao = COALESCE(EXCLUDED.data_inicio_execucao, tarefas.data_inicio_execucao),
		              data_entrada_sprint = EXCLUDED.data_entrada_sprint,
		              marcacao = EXCLUDED.marcacao,
		              campos_extras = EXCLUDED.campos_extras, updated_at = NOW()
		RETURNING id
	`, t.FonteDadosID, t.ProjetoID, t.JiraID, t.NumeroTicket, t.Resumo,
		t.Tipo, t.Status, t.Prioridade, t.EstimativaPontos, t.EstimativaTempo, t.TempoGasto,
		t.ResponsavelID, t.RelatorID, t.Team, t.SprintID, t.DataCriacao, t.DataLimite,
		t.DataResolvido, t.DataAtualizado, t.TipoDemanda, t.DataComponente,
		t.StatusCategoria, ce, t.ParentID, t.Apelido, t.DataInicioExecucao,
		t.DataEntradaSprint, t.Marcacao).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting tarefa %s: %w", t.NumeroTicket, err)
	}
	return id, nil
}

func (r *SyncRepository) SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	if len(presentJiraIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET removido_em = NOW(), motivo_remocao = 'removido do jira'
		WHERE fonte_dados_id = $1
		  AND jira_id != ALL($2)
		  AND removido_em IS NULL
	`, fonteDadosID, presentJiraIDs)
	if err != nil {
		return 0, fmt.Errorf("soft-deleting absent tarefas: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *SyncRepository) UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	if len(presentJiraIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET removido_em = NULL, motivo_remocao = NULL
		WHERE fonte_dados_id = $1
		  AND jira_id = ANY($2)
		  AND removido_em IS NOT NULL
	`, fonteDadosID, presentJiraIDs)
	if err != nil {
		return 0, fmt.Errorf("undeleting reappeared tarefas: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *SyncRepository) LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM tarefas WHERE fonte_dados_id = $1 AND jira_id = $2
	`, fonteDadosID, jiraID).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("looking up tarefa by jira_id %s: %w", jiraID, err)
	}
	return id, nil
}

func (r *SyncRepository) UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET parent_id = $2, updated_at = NOW() WHERE id = $1
	`, tarefaID, parentID)
	if err != nil {
		return fmt.Errorf("updating parent_id for tarefa %s: %w", tarefaID, err)
	}
	return nil
}

func (r *SyncRepository) UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO produtos (id, fonte_dados_id, jira_id, nome, descricao, projeto_id, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, true)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET nome = EXCLUDED.nome, descricao = EXCLUDED.descricao,
		              projeto_id = EXCLUDED.projeto_id, ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, nome, descricao, projetoID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting produto %s: %w", nome, err)
	}
	return id, nil
}

func (r *SyncRepository) LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tarefa_produtos (tarefa_id, produto_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, tarefaID, produtoID)
	if err != nil {
		return fmt.Errorf("linking tarefa %s to produto %s: %w", tarefaID, produtoID, err)
	}
	return nil
}

func (r *SyncRepository) HasRunningSync(ctx context.Context, fonteDadosID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sync_logs
			WHERE fonte_dados_id = $1 AND status = 'running'
		)
	`, fonteDadosID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking running sync: %w", err)
	}
	return exists, nil
}

func (r *SyncRepository) HasRunningSyncForProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sync_logs
			WHERE fonte_dados_id = $1 AND status = 'running' AND project_key = $2
		)
	`, fonteDadosID, projectKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking running sync for project: %w", err)
	}
	return exists, nil
}

func (r *SyncRepository) CreateSyncLog(ctx context.Context, log *domain.SyncLog) error {
	origem := log.Origem
	if origem == "" {
		origem = "manual"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sync_logs (id, fonte_dados_id, tipo, status, iniciado_em, mensagem, origem, project_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, log.ID, log.FonteDadosID, log.Tipo, log.Status, log.IniciadoEm, log.Mensagem, origem, log.ProjectKey)
	if err != nil {
		return fmt.Errorf("creating sync log: %w", err)
	}
	return nil
}

func (r *SyncRepository) UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals SyncTotals) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_logs
		SET total_projetos = $2, total_tarefas = $3, total_membros = $4, total_sprints = $5
		WHERE id = $1
	`, id, totals.Projetos, totals.Tarefas, totals.Membros, totals.Sprints)
	if err != nil {
		return fmt.Errorf("updating sync log totals %s: %w", id, err)
	}
	return nil
}

func (r *SyncRepository) UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals SyncTotals, erros json.RawMessage, mensagem *string) error {
	if erros == nil {
		erros = json.RawMessage(`[]`)
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_logs
		SET status = $2, finalizado_em = $3, total_projetos = $4, total_tarefas = $5,
		    total_membros = $6, total_sprints = $7, erros = $8, mensagem = $9
		WHERE id = $1
	`, id, status, finalizadoEm, totals.Projetos, totals.Tarefas, totals.Membros, totals.Sprints, erros, mensagem)
	if err != nil {
		return fmt.Errorf("updating sync log %s: %w", id, err)
	}
	return nil
}

func (r *SyncRepository) GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	var log domain.SyncLog
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, tipo, status, iniciado_em, finalizado_em,
		       total_projetos, total_tarefas, total_membros, total_sprints,
		       erros, mensagem, created_at, origem
		FROM sync_logs
		WHERE fonte_dados_id = $1
		ORDER BY iniciado_em DESC
		LIMIT 1
	`, fonteDadosID).Scan(
		&log.ID, &log.FonteDadosID, &log.Tipo, &log.Status, &log.IniciadoEm, &log.FinalizadoEm,
		&log.TotalProjetos, &log.TotalTarefas, &log.TotalMembros, &log.TotalSprints,
		&log.Erros, &log.Mensagem, &log.CreatedAt, &log.Origem,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest sync log: %w", err)
	}
	return &log, nil
}

func (r *SyncRepository) ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, tipo, status, iniciado_em, finalizado_em,
		       total_projetos, total_tarefas, total_membros, total_sprints,
		       erros, mensagem, created_at, origem
		FROM sync_logs
		WHERE fonte_dados_id = $1
		ORDER BY iniciado_em DESC
		LIMIT $2
	`, fonteDadosID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sync logs: %w", err)
	}
	defer rows.Close()

	logs := make([]domain.SyncLog, 0)
	for rows.Next() {
		var log domain.SyncLog
		if err := rows.Scan(
			&log.ID, &log.FonteDadosID, &log.Tipo, &log.Status, &log.IniciadoEm, &log.FinalizadoEm,
			&log.TotalProjetos, &log.TotalTarefas, &log.TotalMembros, &log.TotalSprints,
			&log.Erros, &log.Mensagem, &log.CreatedAt, &log.Origem,
		); err != nil {
			return nil, fmt.Errorf("scanning sync log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *SyncRepository) GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT p.chave
		FROM projetos p
		INNER JOIN tarefas t ON t.projeto_id = p.id
		INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id AND em.data_saida IS NULL
		WHERE p.fonte_dados_id = $1 AND p.ativo = true
	`, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting project keys for sync: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scanning project key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *SyncRepository) GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome, tipo, base_url, auth_type, api_token, user_email,
		       oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		       oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		       ativo, ultimo_sync, created_at, updated_at
		FROM fonte_dados
		WHERE ativo = true AND tipo = 'jira'
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("getting active fonte dados: %w", err)
	}
	defer rows.Close()

	result := make([]domain.FonteDados, 0)
	for rows.Next() {
		fd, err := scanFonteDados(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning fonte dados: %w", err)
		}
		result = append(result, fd)
	}
	return result, rows.Err()
}

func (r *SyncRepository) UpdateCustomFieldMap(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error {
	_, err := r.pool.Exec(ctx, `UPDATE fonte_dados SET custom_field_map = $2 WHERE id = $1`, fonteID, cfMap)
	return err
}

func (r *SyncRepository) AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error) {
	rows, err := r.pool.Query(ctx, `
		WITH equipe_boards AS (
			SELECT em.equipe_id, s.board_id, COUNT(*) as cnt,
			       ROW_NUMBER() OVER (PARTITION BY em.equipe_id ORDER BY COUNT(*) DESC, s.board_id ASC) as rn
			FROM sprints s
			JOIN tarefas t ON t.sprint_id = s.id
			JOIN equipe_membros em ON em.membro_id = t.responsavel_id AND em.data_saida IS NULL
			WHERE s.fonte_dados_id = $1 AND s.board_id IS NOT NULL
			GROUP BY em.equipe_id, s.board_id
		)
		UPDATE equipes e
		SET board_id = eb.board_id, updated_at = NOW()
		FROM equipe_boards eb
		WHERE eb.equipe_id = e.id AND eb.rn = 1 AND e.board_id IS NULL
		RETURNING e.id
	`, fonteDadosID)
	if err != nil {
		return 0, fmt.Errorf("auto-detecting equipe board_ids: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
