package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

const horasPorDia = 6.0

type SprintStore interface {
	ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SprintInfo, error)
	GetCapacity(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*SprintCapacityResult, error)
}

type SprintInfo struct {
	ID         uuid.UUID  `json:"id"`
	Nome       string     `json:"nome"`
	Estado     *string    `json:"estado"`
	DataInicio *time.Time `json:"data_inicio"`
	DataFim    *time.Time `json:"data_fim"`
}

type AusenciaInfo struct {
	Tipo       string `json:"tipo"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
	Dias       int    `json:"dias"`
}

type FeriadoInfo struct {
	Data string `json:"data"`
	Nome string `json:"nome"`
}

type TarefaCapacityDetail struct {
	ID           uuid.UUID `json:"id"`
	NumeroTicket string    `json:"numero_ticket"`
	Resumo       string    `json:"resumo"`
	Tipo         string    `json:"tipo"`
	Status       string    `json:"status"`
	Prioridade   *string   `json:"prioridade"`
	Horas        float64   `json:"horas"`
	ProjetoID    uuid.UUID `json:"projeto_id"`
	ProjetoChave string    `json:"projeto_chave"`
	ProjetoNome  string    `json:"projeto_nome"`
}

type MembroCapacity struct {
	MembroID            uuid.UUID              `json:"membro_id"`
	Nome                string                 `json:"nome"`
	AvatarURL           *string                `json:"avatar_url"`
	HorasEstimadas      float64                `json:"horas_estimadas"`
	HorasAlocadas       float64                `json:"horas_alocadas"`
	HorasExecutadas     float64                `json:"horas_executadas"`
	HorasDisponiveis    float64                `json:"horas_disponiveis"`
	PercentualAlocacao  float64                `json:"percentual_alocacao"`
	PercentualExecutado float64                `json:"percentual_executado"`
	Overcapacity        bool                   `json:"overcapacity"`
	DaEquipe            bool                   `json:"da_equipe"`
	Desligado           bool                   `json:"desligado"`
	Ausencias           []AusenciaInfo         `json:"ausencias"`
	Tarefas             []TarefaCapacityDetail `json:"tarefas"`
}

type MembroAusenteComCards struct {
	Nome        string `json:"nome"`
	Motivo      string `json:"motivo"`
	DiasAusente int    `json:"dias_ausente"`
	SprintInteira bool `json:"sprint_inteira"`
}

type SprintCapacityResult struct {
	Sprint                  SprintInfo               `json:"sprint"`
	DiasUteis               int                      `json:"dias_uteis"`
	Feriados                []FeriadoInfo            `json:"feriados"`
	TotalMembrosEquipe      int                      `json:"total_membros_equipe"`
	HorasTotalSprint        float64                  `json:"horas_total_sprint"`
	HorasAlocadas           float64                  `json:"horas_alocadas"`
	HorasExecutadas         float64                  `json:"horas_executadas"`
	HorasPendentesExecucao  float64                  `json:"horas_pendentes_execucao"`
	Membros                 []MembroCapacity         `json:"membros"`
	MembrosAusentesComCards []MembroAusenteComCards   `json:"membros_ausentes_com_cards"`
	FonteDadosID            uuid.UUID                `json:"fonte_dados_id"`
	ProjetoChave            string                   `json:"projeto_chave"`
}

type SprintService struct {
	repo   *repository.SprintRepository
	logger *zap.Logger
}

func NewSprintService(repo *repository.SprintRepository, logger *zap.Logger) *SprintService {
	return &SprintService{repo: repo, logger: logger}
}

func (s *SprintService) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
	return s.repo.ListProjetosComSprints(ctx, equipeID)
}

func (s *SprintService) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return s.repo.ListByProjeto(ctx, projetoID, estado)
}

func (s *SprintService) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return s.repo.ListSprints(ctx, equipeID, estado)
}

func (s *SprintService) GetCapacity(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*SprintCapacityResult, error) {
	sprint, err := s.repo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	info := SprintInfo{
		ID:         sprint.ID,
		Nome:       sprint.Nome,
		Estado:     sprint.Estado,
		DataInicio: sprint.DataInicio,
		DataFim:    sprint.DataFim,
	}

	var projetoChave string
	if sprint.ProjetoID != nil {
		if chave, err := s.repo.GetProjetoChave(ctx, *sprint.ProjetoID); err == nil {
			projetoChave = chave
		}
	}

	emptyResult := &SprintCapacityResult{
		Sprint:       info,
		Feriados:     []FeriadoInfo{},
		Membros:      []MembroCapacity{},
		FonteDadosID: sprint.FonteDadosID,
		ProjetoChave: projetoChave,
	}

	if sprint.DataInicio == nil || sprint.DataFim == nil {
		return emptyResult, nil
	}

	feriados, err := s.repo.GetFeriadosNoPeriodo(ctx, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		return nil, err
	}

	feriadoSet := make(map[string]bool)
	var feriadosInfo []FeriadoInfo
	for _, f := range feriados {
		key := f.Data.Format("2006-01-02")
		d, _ := time.Parse("2006-01-02", key)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			feriadoSet[key] = true
			feriadosInfo = append(feriadosInfo, FeriadoInfo{
				Data: key,
				Nome: f.Nome,
			})
		}
	}
	if feriadosInfo == nil {
		feriadosInfo = []FeriadoInfo{}
	}

	diasUteis := contarDiasUteisComFeriados(*sprint.DataInicio, *sprint.DataFim, feriadoSet)

	var equipeMembroIDs map[uuid.UUID]bool
	var equipeMembrosInfo []repository.MembroInfo
	if equipeID != nil {
		equipeMembroIDs, err = s.repo.GetMembrosEquipeIDs(ctx, *equipeID, *sprint.DataFim)
		if err != nil {
			return nil, err
		}
		equipeMembrosInfo, err = s.repo.GetMembrosEquipeInfo(ctx, *equipeID, *sprint.DataFim)
		if err != nil {
			return nil, err
		}
	}

	membros, err := s.repo.GetMembrosFromSprint(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	membrosSet := make(map[uuid.UUID]bool)
	for _, m := range membros {
		membrosSet[m.ID] = true
	}
	for _, em := range equipeMembrosInfo {
		if !membrosSet[em.ID] {
			membros = append(membros, em)
		}
	}

	if len(membros) == 0 {
		return &SprintCapacityResult{
			Sprint:             info,
			DiasUteis:          diasUteis,
			Feriados:           feriadosInfo,
			TotalMembrosEquipe: len(equipeMembroIDs),
			HorasTotalSprint:   float64(diasUteis) * horasPorDia * float64(len(equipeMembroIDs)),
			Membros:            []MembroCapacity{},
		}, nil
	}

	tarefasDetail, err := s.repo.GetTarefasDetailBySprint(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	statusExecutado := map[string]bool{
		"Code Review": true, "Teste": true, "Validação do Solicitante": true, "Deploy": true, "Concluído": true,
	}
	statusAmbos := map[string]bool{
		"Teste": true, "Validação do Solicitante": true, "Deploy": true,
	}
	statusPendente := map[string]bool{
		"Backlog": true, "Desenvolvimento": true, "Em Desenvolvimento": true, "A Fazer": true,
	}

	horasAlocadasMembro := make(map[uuid.UUID]float64)
	horasExecutadasMembro := make(map[uuid.UUID]float64)
	horasAmbosPorMembro := make(map[uuid.UUID]float64)
	horasPendentesMembro := make(map[uuid.UUID]float64)
	tarefasPorMembro := make(map[uuid.UUID][]TarefaCapacityDetail)
	for _, t := range tarefasDetail {
		if t.Status == "Cancelado" {
			continue
		}
		horas := float64(t.Segundos) / 3600.0
		if statusAmbos[t.Status] {
			horasAlocadasMembro[t.ResponsavelID] += horas
			horasExecutadasMembro[t.ResponsavelID] += horas
			horasAmbosPorMembro[t.ResponsavelID] += horas
		} else if statusExecutado[t.Status] {
			horasExecutadasMembro[t.ResponsavelID] += horas
		} else {
			horasAlocadasMembro[t.ResponsavelID] += horas
			if statusPendente[t.Status] {
				horasPendentesMembro[t.ResponsavelID] += horas
			}
		}
		tarefasPorMembro[t.ResponsavelID] = append(tarefasPorMembro[t.ResponsavelID], TarefaCapacityDetail{
			ID:           t.ID,
			NumeroTicket: t.NumeroTicket,
			Resumo:       t.Resumo,
			Tipo:         t.Tipo,
			Status:       t.Status,
			Prioridade:   t.Prioridade,
			Horas:        math.Round(horas*10) / 10,
			ProjetoID:    t.ProjetoID,
			ProjetoChave: t.ProjetoChave,
			ProjetoNome:  t.ProjetoNome,
		})
	}

	membroIDs := make([]uuid.UUID, len(membros))
	for i, m := range membros {
		membroIDs[i] = m.ID
	}

	ausencias, err := s.repo.GetAusenciasNoPeriodo(ctx, membroIDs, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		return nil, err
	}

	ausenciasPorMembro := make(map[uuid.UUID][]repository.AusenciaRecord)
	for _, a := range ausencias {
		ausenciasPorMembro[a.MembroID] = append(ausenciasPorMembro[a.MembroID], a)
	}

	var horasAlocadasEquipe float64
	var horasExecutadasEquipe float64
	var horasPendentesEquipe float64
	totalMembrosEquipe := 0

	result := make([]MembroCapacity, 0, len(membros))
	for _, m := range membros {
		daEquipe := equipeMembroIDs == nil || equipeMembroIDs[m.ID]
		desligado := m.DataDesligamento != nil && !m.DataDesligamento.After(*sprint.DataFim)

		diasAusencia := 0
		var ausenciasInfo []AusenciaInfo

		for _, a := range ausenciasPorMembro[m.ID] {
			inicio := a.DataInicio
			if inicio.Before(*sprint.DataInicio) {
				inicio = *sprint.DataInicio
			}
			fim := a.DataFim
			if fim.After(*sprint.DataFim) {
				fim = *sprint.DataFim
			}
			dias := contarDiasUteisComFeriados(inicio, fim, feriadoSet)
			diasAusencia += dias
			ausenciasInfo = append(ausenciasInfo, AusenciaInfo{
				Tipo:       a.Tipo,
				DataInicio: a.DataInicio.Format("2006-01-02"),
				DataFim:    a.DataFim.Format("2006-01-02"),
				Dias:       dias,
			})
		}

		diasDisponiveis := diasUteis - diasAusencia
		if diasDisponiveis < 0 {
			diasDisponiveis = 0
		}
		horasDisponiveis := float64(diasDisponiveis) * horasPorDia
		horasAloc := horasAlocadasMembro[m.ID]
		horasExec := horasExecutadasMembro[m.ID]
		horasAmbosMembro := horasAmbosPorMembro[m.ID]
		horasEstimadas := horasAloc + horasExec - horasAmbosMembro

		horasAlocPura := horasAloc - horasAmbosMembro
		var pct float64
		if horasDisponiveis > 0 {
			pct = math.Round((horasAlocPura/horasDisponiveis)*1000) / 10
		} else if horasAlocPura > 0 {
			pct = 999.9
		}

		var pctExec float64
		if horasDisponiveis > 0 {
			pctExec = math.Round((horasExec/horasDisponiveis)*1000) / 10
		}

		if ausenciasInfo == nil {
			ausenciasInfo = []AusenciaInfo{}
		}

		memberTarefas := tarefasPorMembro[m.ID]
		if memberTarefas == nil {
			memberTarefas = []TarefaCapacityDetail{}
		}

		if daEquipe && !desligado {
			horasAlocadasEquipe += math.Round(horasAlocPura*10) / 10
			horasExecutadasEquipe += math.Round(horasExec*10) / 10
			horasPendentesEquipe += math.Round(horasPendentesMembro[m.ID]*10) / 10
			totalMembrosEquipe++
		}

		result = append(result, MembroCapacity{
			MembroID:            m.ID,
			Nome:                m.Nome,
			AvatarURL:           m.AvatarURL,
			HorasEstimadas:      math.Round(horasEstimadas*10) / 10,
			HorasAlocadas:       math.Round(horasAlocPura*10) / 10,
			HorasExecutadas:     math.Round(horasExec*10) / 10,
			HorasDisponiveis:    math.Round(horasDisponiveis*10) / 10,
			PercentualAlocacao:  pct,
			PercentualExecutado: pctExec,
			Overcapacity:        pct > 100,
			DaEquipe:            daEquipe,
			Desligado:           desligado,
			Ausencias:           ausenciasInfo,
			Tarefas:             memberTarefas,
		})
	}

	if equipeMembroIDs != nil && totalMembrosEquipe == 0 {
		totalMembrosEquipe = len(equipeMembroIDs)
	}

	var horasTotalSprint float64
	for _, mc := range result {
		if mc.DaEquipe {
			horasTotalSprint += mc.HorasDisponiveis
		}
	}

	var membrosAusentes []MembroAusenteComCards
	for _, mc := range result {
		if len(mc.Ausencias) == 0 || len(mc.Tarefas) == 0 {
			continue
		}
		totalDias := 0
		motivo := "ausente"
		for _, a := range mc.Ausencias {
			totalDias += a.Dias
			motivo = a.Tipo
		}
		sprintInteira := mc.HorasDisponiveis <= 0
		membrosAusentes = append(membrosAusentes, MembroAusenteComCards{
			Nome:          mc.Nome,
			Motivo:        motivo,
			DiasAusente:   totalDias,
			SprintInteira: sprintInteira,
		})
	}
	if membrosAusentes == nil {
		membrosAusentes = []MembroAusenteComCards{}
	}

	return &SprintCapacityResult{
		Sprint:                  info,
		DiasUteis:               diasUteis,
		Feriados:                feriadosInfo,
		TotalMembrosEquipe:      totalMembrosEquipe,
		HorasTotalSprint:        math.Round(horasTotalSprint*10) / 10,
		HorasAlocadas:           math.Round(horasAlocadasEquipe*10) / 10,
		HorasExecutadas:         math.Round(horasExecutadasEquipe*10) / 10,
		HorasPendentesExecucao:  math.Round(horasPendentesEquipe*10) / 10,
		Membros:                 result,
		MembrosAusentesComCards: membrosAusentes,
		FonteDadosID:            sprint.FonteDadosID,
		ProjetoChave:            projetoChave,
	}, nil
}

type SprintAtualUnplanned struct {
	TotalTarefas            int     `json:"total_tarefas"`
	TarefasNaoPlanejadas    int     `json:"tarefas_nao_planejadas"`
	PercentualNaoPlanejadas float64 `json:"percentual_nao_planejadas"`
	HorasNaoPlanejadas      float64 `json:"horas_nao_planejadas"`
	HorasTotalSprint        float64 `json:"horas_total_sprint"`
	ManutencaoCount         int     `json:"manutencao_count"`
	ManutencaoHoras         float64 `json:"manutencao_horas"`
	OutrasCount             int     `json:"outras_count"`
	OutrasHoras             float64 `json:"outras_horas"`
}

type MediaHistorica struct {
	SprintsAnalisadas         int     `json:"sprints_analisadas"`
	MediaHorasNaoPlanejadas   float64 `json:"media_horas_nao_planejadas"`
	MediaPercentualNaoPlanejadas float64 `json:"media_percentual_nao_planejadas"`
	CapacidadeMediaSprint     float64 `json:"capacidade_media_sprint"`
	PercentualAlocacaoSugerido float64 `json:"percentual_alocacao_sugerido"`
}

type UnplannedAnalysisResult struct {
	SprintAtual    SprintAtualUnplanned `json:"sprint_atual"`
	MediaHistorica MediaHistorica       `json:"media_historica"`
	EquipeNome     string               `json:"equipe_nome"`
}

func (s *SprintService) GetUnplannedAnalysis(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*UnplannedAnalysisResult, error) {
	stats, err := s.repo.GetUnplannedStats(ctx, sprintID, equipeID)
	if err != nil {
		return nil, err
	}

	var pctNaoPlanejadas float64
	if stats.TotalTarefas > 0 {
		pctNaoPlanejadas = math.Round(float64(stats.TarefasNaoPlanejadas)/float64(stats.TotalTarefas)*1000) / 10
	}

	result := &UnplannedAnalysisResult{
		SprintAtual: SprintAtualUnplanned{
			TotalTarefas:            stats.TotalTarefas,
			TarefasNaoPlanejadas:    stats.TarefasNaoPlanejadas,
			PercentualNaoPlanejadas: pctNaoPlanejadas,
			HorasNaoPlanejadas:      math.Round(stats.HorasNaoPlanejadas*10) / 10,
			HorasTotalSprint:        math.Round(stats.HorasTotalSprint*10) / 10,
			ManutencaoCount:         stats.ManutencaoCount,
			ManutencaoHoras:         math.Round(stats.ManutencaoHoras*10) / 10,
			OutrasCount:             stats.OutrasCount,
			OutrasHoras:             math.Round(stats.OutrasHoras*10) / 10,
		},
	}

	var equipeNome string
	if equipeID != nil {
		equipeNome, err = s.repo.GetEquipeNome(ctx, *equipeID)
		if err != nil {
			s.logger.Warn("could not get equipe nome", zap.Error(err))
		}
	}
	result.EquipeNome = equipeNome

	projetoID, err := s.repo.GetSprintProjetoID(ctx, sprintID)
	if err != nil || projetoID == nil {
		return result, nil
	}

	historico, err := s.repo.GetHistoricalUnplanned(ctx, *projetoID, equipeID, sprintID, 8)
	if err != nil {
		s.logger.Warn("could not get historical unplanned", zap.Error(err))
		return result, nil
	}

	if len(historico) == 0 {
		return result, nil
	}

	sprint, err := s.repo.GetByID(ctx, sprintID)
	if err != nil {
		return result, nil
	}

	feriados, err := s.repo.GetFeriadosNoPeriodo(ctx, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		feriados = nil
	}
	feriadoSet := make(map[string]bool)
	for _, f := range feriados {
		key := f.Data.Format("2006-01-02")
		d, _ := time.Parse("2006-01-02", key)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			feriadoSet[key] = true
		}
	}

	var somaHorasNaoPlanejadas float64
	var somaPctNaoPlanejadas float64
	var somaCapacidade float64
	for _, h := range historico {
		somaHorasNaoPlanejadas += h.HorasNaoPlanejadas
		if h.HorasTotal > 0 {
			somaPctNaoPlanejadas += h.HorasNaoPlanejadas / h.HorasTotal * 100
		}
		sprintInfo, err := s.repo.GetByID(ctx, h.SprintID)
		if err != nil || sprintInfo.DataInicio == nil || sprintInfo.DataFim == nil {
			continue
		}
		diasUteis := contarDiasUteisComFeriados(*sprintInfo.DataInicio, *sprintInfo.DataFim, feriadoSet)
		capacidade := float64(diasUteis) * horasPorDia * float64(h.TotalMembros)
		somaCapacidade += capacidade
	}

	n := float64(len(historico))
	mediaHoras := math.Round(somaHorasNaoPlanejadas/n*10) / 10
	mediaPct := math.Round(somaPctNaoPlanejadas/n*10) / 10
	mediaCapacidade := math.Round(somaCapacidade/n*10) / 10

	var pctSugerido float64
	if mediaCapacidade > 0 {
		pctSugerido = math.Round(100 - (mediaHoras/mediaCapacidade*100))
	}
	if pctSugerido < 0 {
		pctSugerido = 0
	}

	result.MediaHistorica = MediaHistorica{
		SprintsAnalisadas:         len(historico),
		MediaHorasNaoPlanejadas:   mediaHoras,
		MediaPercentualNaoPlanejadas: mediaPct,
		CapacidadeMediaSprint:     mediaCapacidade,
		PercentualAlocacaoSugerido: pctSugerido,
	}

	return result, nil
}

func contarDiasUteisComFeriados(inicio, fim time.Time, feriados map[string]bool) int {
	startDate := time.Date(inicio.Year(), inicio.Month(), inicio.Day(), 0, 0, 0, 0, time.UTC)
	endDate := time.Date(fim.Year(), fim.Month(), fim.Day(), 0, 0, 0, 0, time.UTC)

	dias := 0
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if feriados[d.Format("2006-01-02")] {
			continue
		}
		dias++
	}
	return dias
}

type BurndownPoint struct {
	Data  string  `json:"data"`
	Horas float64 `json:"horas"`
}

type BurndownResult struct {
	SprintNome     string          `json:"sprint_nome"`
	DataInicio     string          `json:"data_inicio"`
	DataFim        string          `json:"data_fim"`
	HorasTotal     float64         `json:"horas_total"`
	LinhaIdeal     []BurndownPoint `json:"linha_ideal"`
	LinhaReal      []BurndownPoint `json:"linha_real"`
	LinhaUnplanned []BurndownPoint `json:"linha_nao_planejadas"`
}

func (s *SprintService) GetBurndown(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*BurndownResult, error) {
	sprint, err := s.repo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, err
	}
	if sprint.DataInicio == nil || sprint.DataFim == nil {
		return &BurndownResult{SprintNome: sprint.Nome}, nil
	}

	feriados, err := s.repo.GetFeriadosNoPeriodo(ctx, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		return nil, err
	}
	feriadoSet := make(map[string]bool)
	for _, f := range feriados {
		key := f.Data.Format("2006-01-02")
		d, _ := time.Parse("2006-01-02", key)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			feriadoSet[key] = true
		}
	}

	tarefas, err := s.repo.GetBurndownTarefas(ctx, sprintID, equipeID)
	if err != nil {
		return nil, err
	}

	sprintInicio := sprint.DataInicio.Truncate(24 * time.Hour)
	sprintFim := sprint.DataFim.Truncate(24 * time.Hour)

	var diasUteis []time.Time
	for d := sprintInicio; !d.After(sprintFim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if feriadoSet[d.Format("2006-01-02")] {
			continue
		}
		diasUteis = append(diasUteis, d)
	}

	if len(diasUteis) == 0 {
		return &BurndownResult{SprintNome: sprint.Nome}, nil
	}

	var horasIniciais float64
	for _, t := range tarefas {
		entrou := sprintInicio
		if t.DataEntradaSprint != nil {
			entrou = t.DataEntradaSprint.Truncate(24 * time.Hour)
		}
		if !entrou.After(sprintInicio) {
			horasIniciais += float64(t.EstimativaSegundos) / 3600.0
		}
	}

	ideal := make([]BurndownPoint, len(diasUteis))
	decPerDay := horasIniciais / float64(len(diasUteis)-1)
	for i, d := range diasUteis {
		remaining := horasIniciais - decPerDay*float64(i)
		if i == len(diasUteis)-1 {
			remaining = 0
		}
		ideal[i] = BurndownPoint{
			Data:  d.Format("2006-01-02"),
			Horas: math.Round(remaining*10) / 10,
		}
	}

	status80pct := map[string]bool{
		"Teste": true, "Validação do Solicitante": true, "Deploy": true,
	}

	hoje := time.Now().Truncate(24 * time.Hour)
	var real []BurndownPoint
	horasRestantes := horasIniciais
	for _, d := range diasUteis {
		if d.After(hoje) {
			break
		}
		for _, t := range tarefas {
			horas := float64(t.EstimativaSegundos) / 3600.0
			if t.DataEntradaSprint != nil {
				entrou := t.DataEntradaSprint.Truncate(24 * time.Hour)
				if entrou.Equal(d) && entrou.After(sprintInicio) {
					horasRestantes += horas
				}
			}
			if t.DataResolvido != nil {
				resolvido := t.DataResolvido.Truncate(24 * time.Hour)
				if resolvido.Equal(d) {
					horasRestantes -= horas
				}
			}
		}

		desconto80 := 0.0
		for _, t := range tarefas {
			if t.DataResolvido != nil {
				continue
			}
			if !status80pct[t.Status] {
				continue
			}
			horas := float64(t.EstimativaSegundos) / 3600.0
			desconto80 += horas * 0.8
		}
		horasComDesconto := horasRestantes - desconto80
		if horasComDesconto < 0 {
			horasComDesconto = 0
		}
		real = append(real, BurndownPoint{
			Data:  d.Format("2006-01-02"),
			Horas: math.Round(horasComDesconto*10) / 10,
		})
	}

	var unplanned []BurndownPoint
	horasNaoPlan := 0.0
	for _, d := range diasUteis {
		if d.After(hoje) {
			break
		}
		for _, t := range tarefas {
			entrou := sprintInicio
			if t.DataEntradaSprint != nil {
				entrou = t.DataEntradaSprint.Truncate(24 * time.Hour)
			}
			if !entrou.After(sprintInicio) {
				continue
			}
			horas := float64(t.EstimativaSegundos) / 3600.0
			if entrou.Equal(d) {
				horasNaoPlan += horas
			}
			if t.DataResolvido != nil {
				resolvido := t.DataResolvido.Truncate(24 * time.Hour)
				if resolvido.Equal(d) {
					horasNaoPlan -= horas
				}
			}
		}
		val := horasNaoPlan
		if val < 0 {
			val = 0
		}
		unplanned = append(unplanned, BurndownPoint{
			Data:  d.Format("2006-01-02"),
			Horas: math.Round(val*10) / 10,
		})
	}

	return &BurndownResult{
		SprintNome:     sprint.Nome,
		DataInicio:     sprintInicio.Format("2006-01-02"),
		DataFim:        sprintFim.Format("2006-01-02"),
		HorasTotal:     math.Round(horasIniciais*10) / 10,
		LinhaIdeal:     ideal,
		LinhaReal:      real,
		LinhaUnplanned: unplanned,
	}, nil
}

type SprintTimelineItem struct {
	SprintID            uuid.UUID `json:"sprint_id"`
	SprintNome          string    `json:"sprint_nome"`
	DataInicio          string    `json:"data_inicio"`
	DataFim             string    `json:"data_fim"`
	Estado              string    `json:"estado"`
	HorasMaximoTeorico  float64   `json:"horas_maximo_teorico"`
	HorasCapacidade     float64   `json:"horas_capacidade"`
	HorasAlocadas       float64   `json:"horas_alocadas"`
	Headcount           int       `json:"headcount"`
	FonteDadosID        string    `json:"fonte_dados_id"`
	ProjetoChave        string    `json:"projeto_chave"`
}

func (s *SprintService) GetSprintsTimeline(ctx context.Context, equipeID uuid.UUID, ano int) ([]SprintTimelineItem, error) {
	allSprints, err := s.repo.ListSprintsIncludeEmpty(ctx, &equipeID, nil)
	if err != nil {
		return nil, err
	}

	anoInicio := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	anoFim := time.Date(ano, 12, 31, 23, 59, 59, 0, time.UTC)
	sprints := make([]repository.SprintListItem, 0)
	for _, sp := range allSprints {
		if sp.DataInicio == nil || sp.DataFim == nil {
			continue
		}
		if sp.DataFim.Before(anoInicio) || sp.DataInicio.After(anoFim) {
			continue
		}
		if sp.Estado != nil && *sp.Estado == "closed" {
			continue
		}
		sprints = append(sprints, sp)
	}

	allMembros, err := s.repo.GetAllMembrosEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}

	if len(sprints) == 0 || len(allMembros) == 0 {
		return []SprintTimelineItem{}, nil
	}

	var minDate, maxDate time.Time
	for _, sp := range sprints {
		if sp.DataInicio == nil || sp.DataFim == nil {
			continue
		}
		if minDate.IsZero() || sp.DataInicio.Before(minDate) {
			minDate = *sp.DataInicio
		}
		if maxDate.IsZero() || sp.DataFim.After(maxDate) {
			maxDate = *sp.DataFim
		}
	}

	feriados, err := s.repo.GetFeriadosNoPeriodo(ctx, minDate, maxDate)
	if err != nil {
		return nil, err
	}
	feriadoSet := make(map[string]bool)
	for _, f := range feriados {
		key := f.Data.Format("2006-01-02")
		d, _ := time.Parse("2006-01-02", key)
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			feriadoSet[key] = true
		}
	}

	membroIDs := make([]uuid.UUID, len(allMembros))
	for i, m := range allMembros {
		membroIDs[i] = m.ID
	}

	ausencias, err := s.repo.GetAusenciasNoPeriodo(ctx, membroIDs, minDate, maxDate)
	if err != nil {
		return nil, err
	}

	sprintIDs := make([]uuid.UUID, 0, len(sprints))
	for _, sp := range sprints {
		sprintIDs = append(sprintIDs, sp.ID)
	}

	horasAlocMap, err := s.repo.GetHorasAlocadasPorSprint(ctx, sprintIDs, membroIDs)
	if err != nil {
		return nil, err
	}

	result := make([]SprintTimelineItem, 0, len(sprints))
	for _, sp := range sprints {
		if sp.DataInicio == nil || sp.DataFim == nil {
			continue
		}

		diasUteis := contarDiasUteisComFeriados(*sp.DataInicio, *sp.DataFim, feriadoSet)

		headcount := 0
		var horasCapacidade float64
		for _, m := range allMembros {
			if m.DataDesligamento != nil && !m.DataDesligamento.After(*sp.DataFim) {
				continue
			}
			headcount++

			diasAusencia := 0
			for _, a := range ausencias {
				if a.MembroID != m.ID {
					continue
				}
				inicio := a.DataInicio
				if inicio.Before(*sp.DataInicio) {
					inicio = *sp.DataInicio
				}
				fim := a.DataFim
				if fim.After(*sp.DataFim) {
					fim = *sp.DataFim
				}
				if !inicio.After(fim) {
					diasAusencia += contarDiasUteisComFeriados(inicio, fim, feriadoSet)
				}
			}

			diasDisp := diasUteis - diasAusencia
			if diasDisp < 0 {
				diasDisp = 0
			}
			horasCapacidade += float64(diasDisp) * horasPorDia
		}

		horasMaxTeorico := float64(headcount) * float64(diasUteis) * horasPorDia

		estado := "future"
		if sp.Estado != nil {
			estado = *sp.Estado
		}

		fonteDadosID := ""
		if sp.FonteDadosID != nil {
			fonteDadosID = sp.FonteDadosID.String()
		}
		projetoChave := ""
		if sp.ProjetoChave != nil {
			projetoChave = *sp.ProjetoChave
		}

		item := SprintTimelineItem{
			SprintID:            sp.ID,
			SprintNome:          sp.Nome,
			DataInicio:          sp.DataInicio.Format("2006-01-02"),
			DataFim:             sp.DataFim.Format("2006-01-02"),
			Estado:              estado,
			HorasMaximoTeorico:  math.Round(horasMaxTeorico*10) / 10,
			HorasCapacidade:     math.Round(horasCapacidade*10) / 10,
			HorasAlocadas:       math.Round(horasAlocMap[sp.ID]*10) / 10,
			Headcount:           headcount,
			FonteDadosID:        fonteDadosID,
			ProjetoChave:        projetoChave,
		}
		result = append(result, item)
	}

	return result, nil
}
