package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type WhatsAppProvider struct {
	configRepo ConfigStore
	httpClient *http.Client
	logger     *zap.Logger
}

func NewWhatsAppProvider(configRepo ConfigStore, logger *zap.Logger) *WhatsAppProvider {
	return &WhatsAppProvider{
		configRepo: configRepo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

func (p *WhatsAppProvider) Send(ctx context.Context, number string, text string) error {
	apiURL, err := p.configRepo.GetConfig(ctx, "evolution_api_url")
	if err != nil {
		return fmt.Errorf("evolution_api_url not configured: %w", err)
	}
	apiKey, err := p.configRepo.GetConfig(ctx, "evolution_api_key")
	if err != nil {
		return fmt.Errorf("evolution_api_key not configured: %w", err)
	}
	instance, err := p.configRepo.GetConfig(ctx, "evolution_instance")
	if err != nil {
		return fmt.Errorf("evolution_instance not configured: %w", err)
	}

	body, _ := json.Marshal(map[string]string{
		"number": number,
		"text":   text,
	})

	url := fmt.Sprintf("%s/message/sendText/%s", apiURL, instance)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending whatsapp message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("evolution API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (p *WhatsAppProvider) FormatReviewMessage(data *ReviewData, analise string, sprintNome string) string {
	stats := data.Stats
	total := stats.Total
	if total == 0 {
		total = 1
	}

	pctBugs := stats.BugsIncidentes * 100 / total
	pctMelhorias := stats.MelhoriasInovacoes * 100 / total
	pctOutros := stats.Outros * 100 / total

	planTotal := stats.PlanejadasTotal
	if planTotal == 0 {
		planTotal = 1
	}
	pctPlanConc := stats.PlanejadasConcl * 100 / planTotal

	msg := fmt.Sprintf("📊 *Sprint Review — %s*\n\n", sprintNome)
	msg += fmt.Sprintf("✅ Concluídas: %d/%d (%d%%)\n", stats.Concluidas, stats.Total, stats.Concluidas*100/total)
	msg += fmt.Sprintf("📋 Planejadas Concluídas: %d/%d (%d%%)\n", stats.PlanejadasConcl, stats.PlanejadasTotal, pctPlanConc)
	msg += fmt.Sprintf("🔄 Em Andamento: %d\n", stats.EmAndamento)
	msg += fmt.Sprintf("🐛 Bugs/Incidentes: %d%%\n", pctBugs)
	msg += fmt.Sprintf("🚀 Melhorias/Inovações: %d%%\n", pctMelhorias)
	msg += fmt.Sprintf("📌 Outras Tarefas: %d%%\n", pctOutros)

	if analise != "" {
		msg += fmt.Sprintf("\n💡 *Principais Entregas*\n%s", analise)
	}

	return msg
}
