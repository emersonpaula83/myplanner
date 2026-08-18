package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type EqualizerMembro struct {
	MembroID  uuid.UUID `json:"membro_id"`
	Nome      string    `json:"nome"`
	AvatarURL *string   `json:"avatar_url"`
	PctAntes  float64   `json:"pct_antes"`
	PctDepois float64   `json:"pct_depois"`
}

type EqualizerTarefa struct {
	ID           uuid.UUID `json:"id"`
	NumeroTicket string    `json:"numero_ticket"`
	Resumo       string    `json:"resumo"`
	Horas        float64   `json:"horas"`
	Tipo         string    `json:"tipo"`
	Prioridade   *string   `json:"prioridade"`
}

type EqualizerSugestao struct {
	De                EqualizerMembro   `json:"de"`
	Para              EqualizerMembro   `json:"para"`
	Tarefas           []EqualizerTarefa `json:"tarefas"`
	HorasTransferidas float64           `json:"horas_transferidas"`
	PctTransferido    float64           `json:"pct_transferido"`
	Justificativa     string            `json:"justificativa"`
}

type MembroAntesDepois struct {
	MembroID    uuid.UUID `json:"membro_id"`
	Nome        string    `json:"nome"`
	AvatarURL   *string   `json:"avatar_url"`
	PctAntes    float64   `json:"pct_antes"`
	PctDepois   float64   `json:"pct_depois"`
	HorasAntes  float64   `json:"horas_antes"`
	HorasDepois float64   `json:"horas_depois"`
}

type EqualizerResult struct {
	Sugestoes          []EqualizerSugestao `json:"sugestoes"`
	MembrosAntesDepois []MembroAntesDepois `json:"membros_antes_depois"`
	NadaASugerir       bool                `json:"nada_a_sugerir"`
	Motivo             string              `json:"motivo,omitempty"`
	Analise            string              `json:"analise,omitempty"`
	DesvioPadraoAntes  float64             `json:"desvio_padrao_antes"`
	DesvioPadraoDepois float64             `json:"desvio_padrao_depois"`
}

type TransferRequest struct {
	TarefaID          uuid.UUID `json:"tarefa_id"`
	TarefaKey         string    `json:"tarefa_key"`
	NovoResponsavelID uuid.UUID `json:"novo_responsavel_id"`
}

type ApplyRequest struct {
	SprintID       uuid.UUID         `json:"-"`
	FonteDadosID   uuid.UUID         `json:"fonte_dados_id"`
	Transferencias []TransferRequest `json:"transferencias"`
}

type ApplyResult struct {
	Aplicadas int          `json:"aplicadas"`
	Erros     []ApplyError `json:"erros"`
}

type ApplyError struct {
	TarefaKey string `json:"tarefa_key"`
	Erro      string `json:"erro"`
}

// ---------------------------------------------------------------------------
// Internal per-member state used while validating AI suggestions
// ---------------------------------------------------------------------------

type membroRole int

const (
	roleNeutral  membroRole = iota
	roleDoador              // overcapacity (> 100%): can only donate, never receive
	roleReceptor            // unused by the AI-powered Calculate; kept for backward compatibility
)

type membroState struct {
	mc       MembroCapacity
	horasMov float64 // signed net change in hours: negative when the member is a net donor, positive when a net receiver
	role     membroRole
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type EqualizerService struct {
	sprintSvc          *SprintService
	sprintRepo         *repository.SprintRepository
	fdRepo             FonteDadosStore
	configRepo         ConfigStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewEqualizerService(
	sprintSvc *SprintService,
	sprintRepo *repository.SprintRepository,
	fdRepo FonteDadosStore,
	configRepo ConfigStore,
	clientFactory ClientFactory,
	oauthClientFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *EqualizerService {
	return &EqualizerService{
		sprintSvc:          sprintSvc,
		sprintRepo:         sprintRepo,
		fdRepo:             fdRepo,
		configRepo:         configRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}

// ---------------------------------------------------------------------------
// buildClient — same pattern as SyncService.buildClient
// ---------------------------------------------------------------------------

func (s *EqualizerService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
	fonte, err := s.fdRepo.GetByID(ctx, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting fonte dados: %w", err)
	}

	if fonte.AuthType == "oauth2" {
		if fonte.OAuth2AccessToken == nil || fonte.OAuth2RefreshToken == nil {
			return nil, fmt.Errorf("fonte %s: oauth2 tokens missing", fonte.Nome)
		}
		accessToken := *fonte.OAuth2AccessToken
		if fonte.OAuth2TokenExpiry != nil && time.Now().After(*fonte.OAuth2TokenExpiry) {
			if s.oauthSvc == nil {
				return nil, fmt.Errorf("fonte %s: oauth token expired and no oauth service configured", fonte.Nome)
			}
			newTokens, err := s.oauthSvc.RefreshAccessToken(ctx, *fonte.OAuth2RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("refreshing oauth token for %s: %w", fonte.Nome, err)
			}
			expiry := newTokens.Expiry()
			if err := s.fdRepo.SaveOAuthTokens(ctx, fonte.ID, fonte.BaseURL, newTokens.AccessToken, newTokens.RefreshToken, expiry); err != nil {
				return nil, fmt.Errorf("saving refreshed tokens: %w", err)
			}
			accessToken = newTokens.AccessToken
		}
		return s.oauthClientFactory(fonte.BaseURL, accessToken, s.rateLimit, s.logger), nil
	}

	email := ""
	if fonte.UserEmail != nil {
		email = *fonte.UserEmail
	}
	apiToken := ""
	if fonte.APIToken != nil {
		apiToken = *fonte.APIToken
	}
	return s.clientFactory(fonte.BaseURL, email, apiToken, s.rateLimit, s.logger), nil
}

// ---------------------------------------------------------------------------
// AI prompt builder — used by Calculate to ask the LLM for a suggested set
// of task transfers to equalize member capacity.
// ---------------------------------------------------------------------------

type aiMembroInput struct {
	Nome             string  `json:"nome"`
	PctAlocacao      float64 `json:"pct_alocacao"`
	HorasDisponiveis float64 `json:"horas_disponiveis"`
	HorasAlocadas    float64 `json:"horas_alocadas"`
}

type aiTarefaInput struct {
	Ticket          string  `json:"ticket"`
	Resumo          string  `json:"resumo"`
	Horas           float64 `json:"horas"`
	Tipo            string  `json:"tipo"`
	ResponsavelNome string  `json:"responsavel_nome"`
}

type aiSugestaoOutput struct {
	DeNome        string `json:"de_nome"`
	ParaNome      string `json:"para_nome"`
	TarefaTicket  string `json:"tarefa_ticket"`
	Justificativa string `json:"justificativa"`
}

type equalizerAIResponse struct {
	Sugestoes    []aiSugestaoOutput `json:"sugestoes"`
	Analise      string             `json:"analise"`
	DesvioAntes  float64            `json:"desvio_antes"`
	DesvioDepois float64            `json:"desvio_depois"`
}

func buildEqualizerPrompt(membros []aiMembroInput, tarefas []aiTarefaInput) (string, string) {
	systemPrompt := `Você é um otimizador de alocação de sprint de desenvolvimento de software.
Analise a capacidade dos membros e sugira transferências de tarefas para equalizar a carga de trabalho.

REGRAS OBRIGATÓRIAS:
1. Membros acima de 100% de alocação NUNCA podem receber tarefas, apenas doar
2. Nenhuma transferência pode levar um membro acima de 100% de alocação
3. Não sugira transferências que apenas invertem percentuais entre dois membros sem reduzir o desvio padrão geral
4. Priorize reduzir o desvio padrão global da alocação
5. Máximo 10 transferências
6. Cada transferência move UMA tarefa de um membro para outro
7. Só sugira transferências de tarefas que existem na lista fornecida

Responda APENAS com JSON válido (sem markdown fences) no formato:
{
  "sugestoes": [
    {
      "de_nome": "Nome Completo Doador",
      "para_nome": "Nome Completo Receptor",
      "tarefa_ticket": "PROJ-123",
      "justificativa": "Texto curto explicando o porquê"
    }
  ],
  "analise": "Resumo de 1-2 frases sobre o estado da sprint e as sugestões",
  "desvio_antes": 15.2,
  "desvio_depois": 8.7
}

Se não houver sugestões viáveis, retorne: {"sugestoes": [], "analise": "motivo", "desvio_antes": X, "desvio_depois": X}`

	type promptData struct {
		Membros []aiMembroInput `json:"membros"`
		Tarefas []aiTarefaInput `json:"tarefas"`
	}
	data, _ := json.Marshal(promptData{Membros: membros, Tarefas: tarefas})
	userPrompt := "DADOS DA SPRINT:\n" + string(data)

	return systemPrompt, userPrompt
}

// calcStdDev returns the population standard deviation of values.
func calcStdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}

// resetHorasMov returns a fresh copy of states with horasMov zeroed, so a
// rejected batch of AI suggestions doesn't leak into the before/after report.
func resetHorasMov(states map[uuid.UUID]*membroState) map[uuid.UUID]*membroState {
	fresh := make(map[uuid.UUID]*membroState, len(states))
	for id, st := range states {
		fresh[id] = &membroState{mc: st.mc, role: st.role}
	}
	return fresh
}

// ---------------------------------------------------------------------------
// Calculate — AI-powered capacity equalization. Delegates the optimization
// itself to an LLM (via OpenRouter) but always re-validates every suggestion
// against the hard rules below before accepting it:
//  1. Members above 100% allocation can only donate, never receive.
//  2. No transfer may push the receiver above 100% allocation.
//  3. A batch of suggestions that doesn't reduce the overall standard
//     deviation of allocation is rejected outright (no swap-only shuffles).
// ---------------------------------------------------------------------------

func (s *EqualizerService) Calculate(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*EqualizerResult, error) {
	cap, err := s.sprintSvc.GetCapacity(ctx, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting capacity: %w", err)
	}

	states := make(map[uuid.UUID]*membroState)
	nameToID := make(map[string]uuid.UUID)
	var activeMembros []aiMembroInput
	var allTarefas []aiTarefaInput
	ticketToTarefa := make(map[string]EqualizerTarefa)
	ticketToOwner := make(map[string]uuid.UUID)

	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		st := &membroState{mc: m, role: roleNeutral}
		if m.PercentualAlocacao > 100 {
			st.role = roleDoador
		}
		states[m.MembroID] = st
		nameToID[m.Nome] = m.MembroID

		if m.HorasDisponiveis <= 0 {
			continue
		}

		activeMembros = append(activeMembros, aiMembroInput{
			Nome:             m.Nome,
			PctAlocacao:      m.PercentualAlocacao,
			HorasDisponiveis: m.HorasDisponiveis,
			HorasAlocadas:    m.HorasAlocadas,
		})

		tarefas, err := s.sprintRepo.GetEqualizerTarefas(ctx, sprintID, m.MembroID)
		if err != nil {
			s.logger.Error("getting equalizer tarefas", zap.Error(err))
			continue
		}
		for _, t := range tarefas {
			allTarefas = append(allTarefas, aiTarefaInput{
				Ticket:          t.NumeroTicket,
				Resumo:          t.Resumo,
				Horas:           t.Horas,
				Tipo:            t.Tipo,
				ResponsavelNome: m.Nome,
			})
			ticketToTarefa[t.NumeroTicket] = EqualizerTarefa{
				ID:           t.ID,
				NumeroTicket: t.NumeroTicket,
				Resumo:       t.Resumo,
				Horas:        t.Horas,
				Tipo:         t.Tipo,
				Prioridade:   t.Prioridade,
			}
			ticketToOwner[t.NumeroTicket] = m.MembroID
		}
	}

	if len(activeMembros) < 2 {
		return s.nadaASugerir(cap, states, "Menos de 2 membros ativos na equipe"), nil
	}
	if len(allTarefas) == 0 {
		return s.nadaASugerir(cap, states, "Nenhuma tarefa movível encontrada"), nil
	}

	// Standard deviation of allocation before any transfers.
	var pcts []float64
	for _, m := range activeMembros {
		pcts = append(pcts, m.PctAlocacao)
	}
	stdDevAntes := math.Round(calcStdDev(pcts)*10) / 10

	// AI is not configured: nothing to suggest, but still report the metric.
	apiKey, err := s.configRepo.GetConfig(ctx, "ai_api_key")
	if err != nil || apiKey == "" {
		apiKey, err = s.configRepo.GetConfig(ctx, "openrouter_api_key")
	}
	if err != nil || apiKey == "" {
		s.logger.Warn("AI API key not configured, no equalizer suggestions")
		result := s.nadaASugerir(cap, states, "Serviço de IA não configurado")
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}
	model := "openai/gpt-4o-mini"
	if m, err := s.configRepo.GetConfig(ctx, "ai_model"); err == nil && m != "" {
		model = m
	} else if m, err := s.configRepo.GetConfig(ctx, "openrouter_model"); err == nil && m != "" {
		model = m
	}

	baseURL := "https://openrouter.ai/api/v1"
	if u, err := s.configRepo.GetConfig(ctx, "ai_base_url"); err == nil && u != "" {
		baseURL = u
	}

	systemPrompt, userPrompt := buildEqualizerPrompt(activeMembros, allTarefas)
	client := NewAIClient(apiKey, model, baseURL)
	rawResponse, err := client.ChatCompletion(ctx, systemPrompt, userPrompt)
	if err != nil {
		s.logger.Error("AI equalizer call failed", zap.Error(err))
		result := s.nadaASugerir(cap, states, "Serviço de IA indisponível")
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}

	// Strip markdown fences, same pattern used by ReviewService.GenerateAnalise.
	cleaned := strings.TrimSpace(rawResponse)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				lines = lines[:i]
				break
			}
		}
		cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var aiResp equalizerAIResponse
	if err := json.Unmarshal([]byte(cleaned), &aiResp); err != nil {
		s.logger.Error("AI returned invalid JSON", zap.Error(err), zap.String("raw", cleaned))
		result := s.nadaASugerir(cap, states, "Resposta da IA inválida")
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}

	if len(aiResp.Sugestoes) == 0 {
		result := s.nadaASugerir(cap, states, firstNonEmpty(aiResp.Analise, "IA não encontrou sugestões viáveis"))
		result.Analise = aiResp.Analise
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}

	// Validate every AI suggestion against the hard rules. Suggestions that
	// fail validation are silently dropped rather than failing the request.
	sugestaoMap := make(map[string]*EqualizerSugestao)
	var sugestoes []*EqualizerSugestao
	usedTickets := make(map[string]bool)

	for _, aiSug := range aiResp.Sugestoes {
		if len(sugestoes) >= 10 {
			break
		}

		deID, deOk := nameToID[aiSug.DeNome]
		paraID, paraOk := nameToID[aiSug.ParaNome]
		tarefa, tarefaOk := ticketToTarefa[aiSug.TarefaTicket]
		if !deOk || !paraOk || !tarefaOk || deID == paraID {
			continue
		}

		// The task must genuinely belong to the claimed donor, and can only
		// be transferred once even if the AI mentions it twice.
		if owner, ok := ticketToOwner[aiSug.TarefaTicket]; !ok || owner != deID {
			continue
		}
		if usedTickets[aiSug.TarefaTicket] {
			continue
		}

		deSt := states[deID]
		paraSt := states[paraID]

		// Hard rule 1: a member above 100% allocation can never receive.
		if paraSt.mc.PercentualAlocacao > 100 {
			continue
		}

		// Hard rule 2: the transfer cannot push the receiver above 100%.
		newParaPct := (paraSt.mc.HorasAlocadas + paraSt.horasMov + tarefa.Horas) / paraSt.mc.HorasDisponiveis * 100
		if newParaPct > 100 {
			continue
		}

		usedTickets[aiSug.TarefaTicket] = true

		key := deID.String() + "->" + paraID.String()
		if existing, ok := sugestaoMap[key]; ok {
			existing.Tarefas = append(existing.Tarefas, tarefa)
			existing.HorasTransferidas += tarefa.Horas
			if aiSug.Justificativa != "" {
				existing.Justificativa += "; " + aiSug.Justificativa
			}
		} else {
			sug := &EqualizerSugestao{
				De:                EqualizerMembro{MembroID: deID, Nome: deSt.mc.Nome, AvatarURL: deSt.mc.AvatarURL},
				Para:              EqualizerMembro{MembroID: paraID, Nome: paraSt.mc.Nome, AvatarURL: paraSt.mc.AvatarURL},
				Tarefas:           []EqualizerTarefa{tarefa},
				HorasTransferidas: tarefa.Horas,
				Justificativa:     aiSug.Justificativa,
			}
			sugestaoMap[key] = sug
			sugestoes = append(sugestoes, sug)
		}
		// horasMov is a signed net delta: donor loses hours, receiver gains them.
		deSt.horasMov -= tarefa.Horas
		paraSt.horasMov += tarefa.Horas
	}

	if len(sugestoes) == 0 {
		result := s.nadaASugerir(cap, states, "Nenhuma sugestão da IA passou na validação das regras de negócio")
		result.Analise = aiResp.Analise
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}

	// Standard deviation after applying the validated suggestions.
	var newPcts []float64
	for _, m := range activeMembros {
		st := states[nameToID[m.Nome]]
		newPct := m.PctAlocacao
		if st.mc.HorasDisponiveis > 0 {
			newPct = (st.mc.HorasAlocadas + st.horasMov) / st.mc.HorasDisponiveis * 100
		}
		newPcts = append(newPcts, newPct)
	}
	stdDevDepois := math.Round(calcStdDev(newPcts)*10) / 10

	// Hard rule 3: reject the whole batch if it doesn't actually reduce the
	// overall standard deviation (i.e. it's just shuffling percentages).
	if stdDevDepois >= stdDevAntes {
		result := s.nadaASugerir(cap, resetHorasMov(states), "Sugestões da IA não reduzem o desvio padrão de alocação da equipe")
		result.Analise = aiResp.Analise
		result.DesvioPadraoAntes = stdDevAntes
		result.DesvioPadraoDepois = stdDevAntes
		return result, nil
	}

	// Calculate before/after percentages on each suggestion.
	for _, sug := range sugestoes {
		d := states[sug.De.MembroID]
		r := states[sug.Para.MembroID]
		sug.De.PctAntes = d.mc.PercentualAlocacao
		sug.De.PctDepois = (d.mc.HorasAlocadas + d.horasMov) / d.mc.HorasDisponiveis * 100
		sug.Para.PctAntes = r.mc.PercentualAlocacao
		sug.Para.PctDepois = (r.mc.HorasAlocadas + r.horasMov) / r.mc.HorasDisponiveis * 100
		sug.PctTransferido = sug.HorasTransferidas / d.mc.HorasDisponiveis * 100
	}

	resultSugestoes := make([]EqualizerSugestao, len(sugestoes))
	for i, sp := range sugestoes {
		resultSugestoes[i] = *sp
	}

	membrosAD := s.buildMembrosAntesDepois(cap, states)

	return &EqualizerResult{
		Sugestoes:          resultSugestoes,
		MembrosAntesDepois: membrosAD,
		NadaASugerir:       false,
		Analise:            aiResp.Analise,
		DesvioPadraoAntes:  stdDevAntes,
		DesvioPadraoDepois: stdDevDepois,
	}, nil
}

// firstNonEmpty returns a if non-empty, otherwise b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// nadaASugerir — returns an EqualizerResult indicating no suggestions.
// ---------------------------------------------------------------------------

func (s *EqualizerService) nadaASugerir(cap *SprintCapacityResult, states map[uuid.UUID]*membroState, motivo string) *EqualizerResult {
	return &EqualizerResult{
		Sugestoes:          nil,
		MembrosAntesDepois: s.buildMembrosAntesDepois(cap, states),
		NadaASugerir:       true,
		Motivo:             motivo,
	}
}

// ---------------------------------------------------------------------------
// buildMembrosAntesDepois — builds a before/after snapshot for every active
// team member, applying each member's signed net horasMov delta (negative
// for donors, positive for receivers) on top of their current allocation.
// ---------------------------------------------------------------------------

func (s *EqualizerService) buildMembrosAntesDepois(cap *SprintCapacityResult, states map[uuid.UUID]*membroState) []MembroAntesDepois {
	var result []MembroAntesDepois
	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		st := states[m.MembroID]

		horasDepois := m.HorasAlocadas
		if st != nil {
			// horasMov is a signed net delta (negative = donated, positive = received).
			horasDepois = m.HorasAlocadas + st.horasMov
		}

		pctDepois := m.PercentualAlocacao
		if m.HorasDisponiveis > 0 {
			pctDepois = horasDepois / m.HorasDisponiveis * 100
		}

		result = append(result, MembroAntesDepois{
			MembroID:    m.MembroID,
			Nome:        m.Nome,
			AvatarURL:   m.AvatarURL,
			PctAntes:    m.PercentualAlocacao,
			PctDepois:   pctDepois,
			HorasAntes:  m.HorasAlocadas,
			HorasDepois: horasDepois,
		})
	}
	return result
}

// ---------------------------------------------------------------------------
// Apply — applies the selected transfers to JIRA and the local database.
// ---------------------------------------------------------------------------

func (s *EqualizerService) Apply(ctx context.Context, req ApplyRequest) (*ApplyResult, error) {
	client, err := s.buildClient(ctx, req.FonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("building jira client: %w", err)
	}

	result := &ApplyResult{}

	for _, tr := range req.Transferencias {
		jiraAccountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, tr.NovoResponsavelID)
		if err != nil {
			result.Erros = append(result.Erros, ApplyError{TarefaKey: tr.TarefaKey, Erro: "membro não encontrado"})
			continue
		}

		if err := client.AssignIssue(ctx, tr.TarefaKey, jiraAccountID); err != nil {
			s.logger.Warn("JIRA assign failed", zap.String("key", tr.TarefaKey), zap.Error(err))
			result.Erros = append(result.Erros, ApplyError{TarefaKey: tr.TarefaKey, Erro: fmt.Sprintf("falha ao reatribuir %s no JIRA", tr.TarefaKey)})
			continue
		}

		novoNome := ""
		if n, err := s.getMembroNome(ctx, tr.NovoResponsavelID); err == nil {
			novoNome = n
		}
		if novoNome == "" {
			novoNome = "outro membro"
		}
		comment := fmt.Sprintf("Tarefa transferida para %s via Equalizador de Capacidade", novoNome)
		if err := client.AddComment(ctx, tr.TarefaKey, comment); err != nil {
			s.logger.Warn("failed to add comment", zap.String("key", tr.TarefaKey), zap.Error(err))
		}

		if err := s.sprintRepo.UpdateTarefaResponsavel(ctx, req.SprintID, tr.TarefaID, tr.NovoResponsavelID); err != nil {
			s.logger.Error("failed to update local responsavel", zap.String("key", tr.TarefaKey), zap.Error(err))
		}

		result.Aplicadas++
	}

	return result, nil
}

// getMembroNome resolves a membro UUID to their display name.
func (s *EqualizerService) getMembroNome(ctx context.Context, membroID uuid.UUID) (string, error) {
	var nome string
	err := s.sprintRepo.Pool().QueryRow(ctx, `SELECT nome FROM membros WHERE id = $1`, membroID).Scan(&nome)
	return nome, err
}
