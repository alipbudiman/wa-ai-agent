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
	"net/url"
	"strings"
	"time"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	openai "github.com/sashabaranov/go-openai"
)

var (
	errAIAgentClientRequired = errors.New("ai agent client is not initialized")
	errMessagesRequired      = errors.New("messages are required")
	errPromptRequired        = errors.New("prompt is required")
	errRunIDRequired         = errors.New("run id is required")
)

const (
	defaultBrainBaseURL     = "http://127.0.0.1:8000"
	defaultBrainAPITimeout  = 360 * time.Second
	fallbackTimeoutResponse = "Maaf, proses AI terlalu lama dan mencapai batas waktu. Silakan coba lagi sebentar."
	fallbackErrorResponse   = "Maaf, terjadi kendala saat memproses permintaan ke AI. Silakan coba lagi."

	SUCCESS = "success"
)

type BrainAPI struct {
	httpClient   *http.Client
	baseURL      string
	defaultModel string
}

type chatAgentRequest struct {
	Number       string         `json:"number"`
	Message      string         `json:"message"`
	IsSystem     bool           `json:"is_system"`
	LogLevel     ports.LogLevel `json:"log_level"`
	GroupID      string         `json:"group_id,omitempty"`
	ReplyContent string         `json:"reply_content,omitempty"`
}

type chatAgentResponse struct {
	Status   string `json:"status"`
	Response string `json:"response"`
}

type InjectAIConversationRequest struct {
	Number   string `json:"number"`
	Messages string `json:"messages"`
}

type LangSmithRunReport = ports.LangSmithRunReport
type LangSmithLogReport = ports.LangSmithLogReport
type AIDataStatus = ports.AIDataStatus
type InjectAIConversationResponse = ports.InjectAIConversationResponse

func NewAIAgent(baseURL string) (*BrainAPI, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		normalizedBaseURL = defaultBrainBaseURL
	}

	return &BrainAPI{
		httpClient: &http.Client{
			Timeout: defaultBrainAPITimeout,
		},
		baseURL: normalizedBaseURL,
	}, nil
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func extractPrompt(messages []openai.ChatCompletionMessage) string {
	return messages[0].Content
}

func extractGroupID(messages []openai.ChatCompletionMessage) string {
	return messages[0].Role
}

func extractReplyMessage(messages []openai.ChatCompletionMessage) string {
	return messages[0].ReasoningContent
}

func (o *BrainAPI) resolveAPIv1Endpoint(path string) string {
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		return o.baseURL + path
	}

	return o.baseURL + "/api/v1" + path
}

func (o *BrainAPI) doGETJSON(ctx context.Context, endpoint string, target any) error {
	if o == nil || o.httpClient == nil {
		return errAIAgentClientRequired
	}

	requestCtx := ctx
	if requestCtx == nil {
		requestCtx = context.Background()
	}

	var cancel context.CancelFunc
	if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
		requestCtx, cancel = context.WithTimeout(requestCtx, defaultBrainAPITimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build get request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute get request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("request failed: status=%d, read_body_error=%w", resp.StatusCode, readErr)
		}

		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return fmt.Errorf("request failed: status=%d", resp.StatusCode)
		}

		return fmt.Errorf("request failed: status=%d, body=%s", resp.StatusCode, bodyText)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func (o *BrainAPI) ChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessage, number string, is_system bool, log_level ports.LogLevel, routing ...string) (string, error) {
	Routing := "chat-agent"
	if len(routing) > 0 {
		Routing = routing[0]
	}

	if o == nil || o.httpClient == nil {
		return "", errAIAgentClientRequired
	}

	if len(messages) == 0 {
		return "", errMessagesRequired
	}

	payload, err := json.Marshal(chatAgentRequest{
		Message:      extractPrompt(messages),
		Number:       number,
		IsSystem:     is_system,
		LogLevel:     log_level,
		GroupID:      extractGroupID(messages),
		ReplyContent: extractReplyMessage(messages),
	})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint := o.baseURL + "/api/v1/" + Routing
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		endpoint = o.baseURL + "/" + Routing
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

	resp, err := o.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return fallbackTimeoutResponse, nil
		}
		return fmt.Sprintf("%s, Error Details:%s", fallbackErrorResponse, err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusGatewayTimeout {
			return fallbackTimeoutResponse, nil
		}

		if readErr != nil {
			return fmt.Sprintf("%s, Error Details:status=%d, read_body_error=%s", fallbackErrorResponse, resp.StatusCode, readErr.Error()), nil
		}

		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return fmt.Sprintf("%s, Error Details:status=%d", fallbackErrorResponse, resp.StatusCode), nil
		}

		return fmt.Sprintf("%s, Error Details:status=%d, body=%s", fallbackErrorResponse, resp.StatusCode, bodyText), nil
	}

	var data chatAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Sprintf("%s, Error Details:%s", fallbackErrorResponse, err.Error()), nil
	}

	responseText := strings.TrimSpace(data.Status)
	if responseText == "" {
		return fallbackErrorResponse, nil
	}

	return responseText, nil
}

func (o *BrainAPI) GetLangSmithRuns(ctx context.Context) ([]LangSmithRunReport, error) {
	endpoint := o.resolveAPIv1Endpoint("/langsmith/runs")

	var runs []LangSmithRunReport
	if err := o.doGETJSON(ctx, endpoint, &runs); err != nil {
		return nil, fmt.Errorf("get langsmith runs: %w", err)
	}

	return runs, nil
}

func (o *BrainAPI) GetLangSmithSelectRun(ctx context.Context, runID string) (*LangSmithLogReport, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, errRunIDRequired
	}

	endpoint := o.resolveAPIv1Endpoint("/langsmith/runs/" + url.PathEscape(runID))

	var runDetail LangSmithLogReport
	if err := o.doGETJSON(ctx, endpoint, &runDetail); err != nil {
		return nil, fmt.Errorf("get langsmith run detail: %w", err)
	}

	return &runDetail, nil
}

func (o *BrainAPI) GetAIDataStatus(ctx context.Context) (*AIDataStatus, error) {
	endpoint := o.baseURL + "/api/v1/" + "aidata/providers"
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		endpoint = o.baseURL + "/" + "aidata/providers"
	}

	var status AIDataStatus
	if err := o.doGETJSON(ctx, endpoint, &status); err != nil {
		return nil, fmt.Errorf("get ai data status: %w", err)
	}

	return &status, nil
}

func (o *BrainAPI) InjectAIConversation(ctx context.Context, number string, messages string) (bool, error) {
	endpoint := o.baseURL + "/api/v1/" + "aidata/inject-conversation"
	if strings.HasSuffix(o.baseURL, "/api/v1") {
		endpoint = o.baseURL + "/" + "aidata/inject-conversation"
	}

	payload := InjectAIConversationRequest{
		Number:   number,
		Messages: messages,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal inject conversation request: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
		requestCtx, cancel = context.WithTimeout(ctx, defaultBrainAPITimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("build inject conversation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("execute inject conversation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return false, fmt.Errorf("inject conversation request failed: status=%d, read_body_error=%w", resp.StatusCode, readErr)
		}

		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return false, fmt.Errorf("inject conversation request failed: status=%d", resp.StatusCode)
		}

		return false, fmt.Errorf("inject conversation request failed: status=%d, body=%s", resp.StatusCode, bodyText)
	}

	var injectResponse InjectAIConversationResponse
	if err := json.NewDecoder(resp.Body).Decode(&injectResponse); err != nil {
		return false, fmt.Errorf("decode inject conversation response: %w", err)
	}

	return injectResponse.Success, nil
}
