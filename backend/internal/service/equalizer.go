package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
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
// Internal state for the greedy algorithm
// ---------------------------------------------------------------------------

type membroRole int

const (
	roleNeutral  membroRole = iota
	roleDoador              // overcapacity (> 100%)
	roleReceptor            // undercapacity (< 70%)
)

type membroState struct {
	mc       MembroCapacity
	horasMov float64     // hours moved: donated OUT for doadores, received IN for receptores
	role     membroRole
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type EqualizerService struct {
	sprintSvc          *SprintService
	sprintRepo         *repository.SprintRepository
	fdRepo             *repository.FonteDadosRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewEqualizerService(
	sprintSvc *SprintService,
	sprintRepo *repository.SprintRepository,
	fdRepo *repository.FonteDadosRepository,
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
// Calculate — greedy capacity equalization algorithm
// ---------------------------------------------------------------------------

func (s *EqualizerService) Calculate(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*EqualizerResult, error) {
	cap, err := s.sprintSvc.GetCapacity(ctx, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting capacity: %w", err)
	}

	states := make(map[uuid.UUID]*membroState)
	var doadores, receptores []uuid.UUID

	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		st := &membroState{mc: m, role: roleNeutral}
		if m.PercentualAlocacao > 100 {
			st.role = roleDoador
			doadores = append(doadores, m.MembroID)
		} else if m.PercentualAlocacao < 70 {
			st.role = roleReceptor
			receptores = append(receptores, m.MembroID)
		}
		states[m.MembroID] = st
	}

	if len(doadores) == 0 {
		return s.nadaASugerir(cap, states, "Nenhum membro com alocação acima de 100%"), nil
	}
	if len(receptores) == 0 {
		return s.nadaASugerir(cap, states, "Nenhum membro com alocação abaixo de 70%"), nil
	}

	// Sort doadores by highest allocation first, receptores by lowest first.
	sort.Slice(doadores, func(i, j int) bool {
		return states[doadores[i]].mc.PercentualAlocacao > states[doadores[j]].mc.PercentualAlocacao
	})
	sort.Slice(receptores, func(i, j int) bool {
		return states[receptores[i]].mc.PercentualAlocacao < states[receptores[j]].mc.PercentualAlocacao
	})

	// Use pointers in the slice so mutations via sugestaoMap are reflected
	// without needing a separate copy-back step.
	var sugestoes []*EqualizerSugestao
	sugestaoMap := make(map[string]*EqualizerSugestao)

	for _, dID := range doadores {
		if len(sugestoes) >= 10 {
			break
		}
		d := states[dID]
		if d.mc.HorasDisponiveis <= 0 {
			continue
		}

		tarefas, err := s.sprintRepo.GetEqualizerTarefas(ctx, sprintID, dID)
		if err != nil {
			s.logger.Error("getting equalizer tarefas", zap.Error(err))
			continue
		}
		if len(tarefas) == 0 {
			continue
		}

		for _, t := range tarefas {
			if len(sugestoes) >= 10 {
				break
			}
			pctShift := t.Horas / d.mc.HorasDisponiveis * 100
			if pctShift < 10 {
				continue
			}

			// Find the best receptor: highest remaining capacity, won't
			// exceed 100% allocation after receiving this task.
			var bestR uuid.UUID
			bestDisp := -1.0
			for _, rID := range receptores {
				r := states[rID]
				if r.mc.HorasDisponiveis <= 0 {
					continue
				}
				disp := r.mc.HorasDisponiveis - r.horasMov
				newPct := (r.mc.HorasAlocadas + r.horasMov + t.Horas) / r.mc.HorasDisponiveis * 100
				if disp > bestDisp && newPct <= 100 {
					bestDisp = disp
					bestR = rID
				}
			}
			if bestR == uuid.Nil {
				continue
			}

			et := EqualizerTarefa{
				ID:           t.ID,
				NumeroTicket: t.NumeroTicket,
				Resumo:       t.Resumo,
				Horas:        t.Horas,
				Tipo:         t.Tipo,
				Prioridade:   t.Prioridade,
			}

			key := dID.String() + "->" + bestR.String()
			if existing, ok := sugestaoMap[key]; ok {
				existing.Tarefas = append(existing.Tarefas, et)
				existing.HorasTransferidas += t.Horas
			} else {
				sug := &EqualizerSugestao{
					De:   EqualizerMembro{MembroID: dID, Nome: d.mc.Nome, AvatarURL: d.mc.AvatarURL},
					Para: EqualizerMembro{MembroID: bestR, Nome: states[bestR].mc.Nome, AvatarURL: states[bestR].mc.AvatarURL},
					Tarefas: []EqualizerTarefa{et},
					HorasTransferidas: t.Horas,
				}
				sugestaoMap[key] = sug
				sugestoes = append(sugestoes, sug)
			}
			d.horasMov += t.Horas
			states[bestR].horasMov += t.Horas
		}
	}

	if len(sugestoes) == 0 {
		return s.nadaASugerir(cap, states, "Nenhuma transferência viável atinge o limiar mínimo de 10%"), nil
	}

	// Calculate before/after percentages on each suggestion.
	for _, sug := range sugestoes {
		d := states[sug.De.MembroID]
		r := states[sug.Para.MembroID]
		sug.De.PctAntes = d.mc.PercentualAlocacao
		sug.De.PctDepois = (d.mc.HorasAlocadas - d.horasMov) / d.mc.HorasDisponiveis * 100
		sug.Para.PctAntes = r.mc.PercentualAlocacao
		sug.Para.PctDepois = (r.mc.HorasAlocadas + r.horasMov) / r.mc.HorasDisponiveis * 100
		sug.PctTransferido = sug.HorasTransferidas / d.mc.HorasDisponiveis * 100
	}

	// Dereference pointers into value slice for the result.
	resultSugestoes := make([]EqualizerSugestao, len(sugestoes))
	for i, sp := range sugestoes {
		resultSugestoes[i] = *sp
	}

	membrosAD := s.buildMembrosAntesDepois(cap, states)

	return &EqualizerResult{
		Sugestoes:          resultSugestoes,
		MembrosAntesDepois: membrosAD,
		NadaASugerir:       false,
	}, nil
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
// team member, using the explicit role to decide the direction of horasMov.
// ---------------------------------------------------------------------------

func (s *EqualizerService) buildMembrosAntesDepois(cap *SprintCapacityResult, states map[uuid.UUID]*membroState) []MembroAntesDepois {
	var result []MembroAntesDepois
	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		st := states[m.MembroID]

		horasDepois := m.HorasAlocadas
		switch st.role {
		case roleDoador:
			horasDepois = m.HorasAlocadas - st.horasMov
		case roleReceptor:
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
