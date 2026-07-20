package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type AnalisadorCapacidade interface {
	Analisar(ctx context.Context, input domain.AnaliseCapacidadeInput) (string, error)
}

type GeminiAnalyzer struct {
	apiKey  string
	model   string
	baseURL string
}

func NewGeminiAnalyzer(apiKey, model string) *GeminiAnalyzer {
	return &GeminiAnalyzer{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (g *GeminiAnalyzer) Analisar(ctx context.Context, input domain.AnaliseCapacidadeInput) (string, error) {
	prompt := buildPrompt(input)

	result, err := g.callAPI(ctx, prompt)
	if err != nil {
		result, err = g.callAPI(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("gemini analysis failed after retry: %w", err)
		}
	}
	return result, nil
}

func (g *GeminiAnalyzer) callAPI(ctx context.Context, prompt string) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling gemini: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini returned %d: %s", resp.StatusCode, string(respBody))
	}

	var gemResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from gemini")
	}

	return gemResp.Candidates[0].Content.Parts[0].Text, nil
}

func buildPrompt(input domain.AnaliseCapacidadeInput) string {
	prompt := fmt.Sprintf(`Você é um analista de capacidade de equipe de desenvolvimento de software.

Equipe: %s
Período: %s/%d
Horas disponíveis no mês: %.0f

`, input.Equipe, nomeMes(input.Mes), input.Ano,
		input.HorasDisponiveis)

	if len(input.MembrosAusentes) > 0 {
		prompt += "Ausências no mês:\n"
		for _, a := range input.MembrosAusentes {
			prompt += fmt.Sprintf("- %s: %s (%d dias úteis)\n", a.Nome, a.Tipo, a.Dias)
		}
		prompt += "\n"
	}

	prompt += "Forneça um diagnóstico breve (3-4 frases) sobre a situação de capacidade da equipe neste mês e uma recomendação acionável."
	return prompt
}

func nomeMes(mes int) string {
	nomes := [13]string{"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	if mes >= 1 && mes <= 12 {
		return nomes[mes]
	}
	return fmt.Sprintf("Mês %d", mes)
}
