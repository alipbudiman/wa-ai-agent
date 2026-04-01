package ai_providers_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

var (
	errAIAgentClientRequired = errors.New("ai agent client is not initialized")
	errMessagesRequired      = errors.New("messages are required")
	errPromptRequired        = errors.New("prompt is required")
	errEmptyResponse         = errors.New("brain api returned empty response")
)

type OpenRouter struct {
	httpClient   *http.Client
	baseURL      string
	defaultModel string
}

type chatAgentRequest struct {
	Message string `json:"message"`
}

type chatAgentResponse struct {
	Status   string `json:"status"`
	Response string `json:"response"`
}

func NewAIAgent(_ string, baseURL string, defaultModel string) (*OpenRouter, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		normalizedBaseURL = "http://127.0.0.1:8000"
	}

	return &OpenRouter{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:      normalizedBaseURL,
		defaultModel: strings.TrimSpace(defaultModel),
	}, nil
}

func (o *OpenRouter) ChatCompletion(ctx context.Context, model string, messages []openai.ChatCompletionMessage) (string, error) {
	if o == nil || o.httpClient == nil {
		return "", errAIAgentClientRequired
	}

	if len(messages) == 0 {
		return "", errMessagesRequired
	}

	prompt := ""
	for index := len(messages) - 1; index >= 0; index-- {
		candidate := strings.TrimSpace(messages[index].Content)
		if candidate == "" {
			continue
		}

		if messages[index].Role == openai.ChatMessageRoleUser {
			prompt = candidate
			break
		}

		if prompt == "" {
			prompt = candidate
		}
	}
	if prompt == "" {
		return "", errPromptRequired
	}

	selectedModel := strings.TrimSpace(model)
	if selectedModel == "" {
		selectedModel = o.defaultModel
	}

	payload, err := json.Marshal(chatAgentRequest{Message: prompt})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint := o.baseURL + "/api/v1/chat-agent"
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		endpoint = o.baseURL + "/chat-agent"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if selectedModel != "" {
		req.Header.Set("X-Model", selectedModel)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call brain api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("brain api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data chatAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("decode brain api response: %w", err)
	}

	responseText := strings.TrimSpace(data.Response)
	if responseText == "" {
		return "", errEmptyResponse
	}

	return responseText, nil
}
