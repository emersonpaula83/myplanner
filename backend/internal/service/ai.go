package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AIClient struct {
	apiKey  string
	model   string
	baseURL string
}

func NewAIClient(apiKey, model, baseURL string) *AIClient {
	return &AIClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}
}

type aiRequest struct {
	Model    string      `json:"model"`
	Messages []aiMessage `json:"messages"`
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (c *AIClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	result, err := c.callAPI(ctx, systemPrompt, userPrompt)
	if err != nil {
		result, err = c.callAPI(ctx, systemPrompt, userPrompt)
		if err != nil {
			return "", fmt.Errorf("AI provider failed after retry: %w", err)
		}
	}
	return result, nil
}

func (c *AIClient) callAPI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := aiRequest{
		Model: c.model,
		Messages: []aiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling AI provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	var aiResp aiResponse
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if aiResp.Error != nil {
		return "", fmt.Errorf("AI provider error: %s", aiResp.Error.Message)
	}

	if len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from AI provider")
	}

	return aiResp.Choices[0].Message.Content, nil
}
