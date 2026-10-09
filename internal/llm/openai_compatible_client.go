package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAICompatibleClient(baseURL, apiKey, model string) *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type chatCompletionsRequest struct {
	Model       string                   `json:"model"`
	Temperature float64                  `json:"temperature"`
	Messages    []chatCompletionsMessage `json:"messages"`
}

type chatCompletionsMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *OpenAICompatibleClient) GeneratePatch(ctx context.Context, req PatchRequest) (PatchResponse, error) {
	system, user := BuildPatchPrompt(req)

	payload := chatCompletionsRequest{
		Model:       c.model,
		Temperature: 0.1,
		Messages: []chatCompletionsMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PatchResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return PatchResponse{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return PatchResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PatchResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PatchResponse{}, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var decoded chatCompletionsResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return PatchResponse{}, fmt.Errorf("decode llm response: %w", err)
	}

	if len(decoded.Choices) == 0 {
		return PatchResponse{}, fmt.Errorf("llm returned no choices")
	}

	content := stripJSONFences(strings.TrimSpace(decoded.Choices[0].Message.Content))

	var out PatchResponse
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return PatchResponse{}, fmt.Errorf("decode patch json: %w; raw=%s", err, content)
	}

	if strings.TrimSpace(out.UpdatedFunction) == "" {
		return PatchResponse{}, fmt.Errorf("llm returned empty updated_function")
	}

	return out, nil
}

func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
