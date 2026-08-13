package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type NotificationService struct {
	reviewSvc    *ReviewService
	destRepo     DestinatarioStore
	sprintRepo   SprintRepoStore
	emailProv    *EmailProvider
	whatsappProv *WhatsAppProvider
	logger       *zap.Logger
}

func NewNotificationService(
	reviewSvc *ReviewService,
	destRepo DestinatarioStore,
	sprintRepo SprintRepoStore,
	emailProv *EmailProvider,
	whatsappProv *WhatsAppProvider,
	logger *zap.Logger,
) *NotificationService {
	return &NotificationService{
		reviewSvc:    reviewSvc,
		destRepo:     destRepo,
		sprintRepo:   sprintRepo,
		emailProv:    emailProv,
		whatsappProv: whatsappProv,
		logger:       logger,
	}
}

type EnvioResultado struct {
	DestinatarioID uuid.UUID `json:"destinatario_id"`
	Tipo           string    `json:"tipo"`
	Status         string    `json:"status"`
	Erro           string    `json:"erro,omitempty"`
}

func (s *NotificationService) EnviarReview(ctx context.Context, sprintID, equipeID uuid.UUID, destIDs []uuid.UUID) ([]EnvioResultado, error) {
	dests, err := s.destRepo.GetByIDs(ctx, destIDs)
	if err != nil {
		return nil, fmt.Errorf("getting destinatarios: %w", err)
	}
	if len(dests) == 0 {
		return nil, fmt.Errorf("nenhum destinatário encontrado")
	}
	for _, d := range dests {
		if d.EquipeID != equipeID {
			return nil, fmt.Errorf("destinatário %s não pertence à equipe informada", d.ID)
		}
	}

	reviewData, err := s.reviewSvc.GetReviewData(ctx, sprintID, equipeID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting review data: %w", err)
	}

	sprintNome := fmt.Sprintf("Sprint %s", sprintID.String()[:8])
	if sprint, err := s.sprintRepo.GetByID(ctx, sprintID); err == nil {
		sprintNome = sprint.Nome
	}

	var analise string
	if a, err := s.reviewSvc.GetAnalise(ctx, sprintID, equipeID, nil); err == nil && a != nil {
		analise = extractAnaliseText(a.AnaliseJSON)
	}

	var resultados []EnvioResultado

	for _, d := range dests {
		r := EnvioResultado{DestinatarioID: d.ID, Tipo: d.Tipo}

		switch d.Tipo {
		case "email":
			htmlBody, err := s.emailProv.RenderReviewEmail(reviewData, analise, sprintNome)
			if err != nil {
				r.Status = "erro"
				r.Erro = fmt.Sprintf("render: %s", err.Error())
				resultados = append(resultados, r)
				continue
			}
			subject := fmt.Sprintf("📊 Sprint Review — %s", sprintNome)
			if err := s.emailProv.Send(ctx, d.Valor, subject, htmlBody); err != nil {
				r.Status = "erro"
				r.Erro = err.Error()
			} else {
				r.Status = "enviado"
			}

		case "whatsapp":
			text := s.whatsappProv.FormatReviewMessage(reviewData, analise, sprintNome)
			if err := s.whatsappProv.Send(ctx, d.Valor, text); err != nil {
				r.Status = "erro"
				r.Erro = err.Error()
			} else {
				r.Status = "enviado"
			}
		}

		resultados = append(resultados, r)
	}

	return resultados, nil
}

func extractAnaliseText(raw json.RawMessage) string {
	var parsed struct {
		AnalisesPorProduto []struct {
			Produto    string `json:"produto"`
			FocoSprint struct {
				Descricao string `json:"descricao"`
			} `json:"foco_sprint"`
			Top3Entregas []struct {
				Ticket string `json:"ticket"`
				Resumo string `json:"resumo"`
			} `json:"top3_entregas"`
		} `json:"analises_por_produto"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.AnalisesPorProduto) == 0 {
		return string(raw)
	}
	var sb strings.Builder
	for _, a := range parsed.AnalisesPorProduto {
		sb.WriteString(fmt.Sprintf("*%s*: %s\n", a.Produto, a.FocoSprint.Descricao))
		for _, e := range a.Top3Entregas {
			sb.WriteString(fmt.Sprintf("  • %s — %s\n", e.Ticket, e.Resumo))
		}
	}
	return sb.String()
}
