package ai_providers_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

var (
	errAIAgentClientRequired = errors.New("ai agent client is not initialized")
	errMessagesRequired      = errors.New("messages are required")
	errPromptRequired        = errors.New("prompt is required")
)

const (
	defaultBrainBaseURL     = "http://127.0.0.1:8000"
	defaultBrainAPITimeout  = 360 * time.Second
	fallbackTimeoutResponse = "Maaf, proses AI terlalu lama dan mencapai batas waktu. Silakan coba lagi sebentar."
	fallbackErrorResponse   = "Maaf, terjadi kendala saat memproses permintaan ke AI. Silakan coba lagi."
)

type OpenRouter struct {
	httpClient   *http.Client
	baseURL      string
	defaultModel string
}

type chatAgentRequest struct {
	UserID string `json:"user_id"`
	Message string `json:"message"`
}

type chatAgentResponse struct {
	Status   string `json:"status"`
	Response string `json:"response"`
}

func NewAIAgent(_ string, baseURL string, defaultModel string) (*OpenRouter, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		normalizedBaseURL = defaultBrainBaseURL
	}

	return &OpenRouter{
		httpClient: &http.Client{
			Timeout: defaultBrainAPITimeout,
		},
		baseURL:      normalizedBaseURL,
		defaultModel: strings.TrimSpace(defaultModel),
	}, nil
}

func (o *OpenRouter) ChatCompletion(ctx context.Context, model string, messages []openai.ChatCompletionMessage, user_id string) (string, error) {
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

	payload, err := json.Marshal(chatAgentRequest{
		Message: prompt, 
		UserID: user_id,
	})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint := o.baseURL + "/api/v1/chat-agent"
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		endpoint = o.baseURL + "/chat-agent"
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
		requestCtx, cancel = context.WithTimeout(ctx, defaultBrainAPITimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
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
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return fallbackTimeoutResponse, nil
		}
		return fmt.Sprintf("%s, Error Details:%s", fallbackErrorResponse, err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, err = io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusGatewayTimeout {
			return fallbackTimeoutResponse, nil
		}
		return fmt.Sprintf("%s, Error Details:%s", fallbackErrorResponse, err.Error()), nil
	}

	var data chatAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Sprintf("%s, Error Details:%s", fallbackErrorResponse, err.Error()), nil
	}

	responseText := strings.TrimSpace(data.Response)
	if responseText == "" {
		return fallbackErrorResponse, nil
	}

	return responseText, nil
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
