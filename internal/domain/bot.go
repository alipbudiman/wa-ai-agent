package ports

import (
	"context"

	"github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	DEBUG   LogLevel = "DEBUG"
	INFO    LogLevel = "INFO"
	WARNING LogLevel = "WARNING"
	ERROR   LogLevel = "ERROR"
	HISTORY LogLevel = "HISTORY"
	NONE    LogLevel = "NONE"
)

type LogLevel string

type ContactCard struct {
	Name     string
	FullName string
	Number   string
}

type BotRunner interface {
	Run(ctx context.Context) error
}

type MessageHandler interface {
	HandleMessageEvent(v *events.Message)
}

type AgentResponder interface {
	Reply(ctx context.Context, prompt []openai.ChatCompletionMessage, is_system bool, log_level LogLevel, jid types.JID, routing ...string) (string, error)
}

type WhatsAppClient interface {
	GetContact(ctx context.Context, number string) (types.ContactInfo, error)
	GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error)
	SendText(ctx context.Context, to string, message *waE2E.Message) error
	SendFile(ctx context.Context, to string, filePath string, caption string) error
	SendContact(ctx context.Context, to string, target ContactCard, context_info *waE2E.ContextInfo) error
	ParseJID(target string) (types.JID, error)
	SendQuoteMessage(ctx context.Context, to, text, quote_msg string) error
	GetSelfJID(ctx context.Context) (types.JID, error)
	// GetAltJID resolves the alternate JID (LID <-> phone-number) for the given
	// JID from the device's mapping store. Used to recognize the bot when a
	// group addresses it in the @lid space instead of @s.whatsapp.net.
	GetAltJID(ctx context.Context, jid types.JID) (types.JID, error)
}

type ChatCompleter interface {
	ChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessage, user_id string, is_system bool, log_level LogLevel, routing ...string) (string, error)
	GetLangSmithRuns(ctx context.Context) ([]LangSmithRunReport, error)
	GetLangSmithSelectRun(ctx context.Context, runID string) (*LangSmithLogReport, error)
	GetAIDataStatus(ctx context.Context) (*AIDataStatus, error)
	InjectAIConversation(ctx context.Context, conversation string, senderNumber string) (bool, error)
	GetAgentScore(ctx context.Context, agentName string, count int) (*AgentScoreReport, error)
	ClearAgentScore(ctx context.Context, agentSpec string) (int, error)
	GetAgentScoreRaw(ctx context.Context, agentSpec string, count int) ([]byte, error)
}

// AgentScoreAverages holds the mean RAGAS Faithfulness and cosine-similarity
// scores across the evaluated logs (nil when no log could be scored), plus the
// aspect pass-rates and the cheap indicators (latency, length, tool calls).
type AgentScoreAverages struct {
	RAGASFaithfulness *float64            `json:"ragas_faithfulness"`
	CosineSimilarity  *float64            `json:"cosine_similarity"`
	Aspects           map[string]*float64 `json:"aspects"`
	LatencyMs         *float64            `json:"latency_ms"`
	ResponseLength    *float64            `json:"response_length"`
	ToolCallCount     *float64            `json:"tool_call_count"`
}

// AgentScoreLog is one evaluated agent response.
type AgentScoreLog struct {
	Timestamp         string          `json:"timestamp"`
	Question          string          `json:"question"`
	Response          string          `json:"response"`
	RAGASFaithfulness *float64        `json:"ragas_faithfulness"`
	CosineSimilarity  *float64        `json:"cosine_similarity"`
	Aspects           map[string]*int `json:"aspects"`
	LatencyMs         *float64        `json:"latency_ms"`
	ResponseLength    *float64        `json:"response_length"`
	ToolCallCount     *float64        `json:"tool_call_count"`
}

// AgentScoreReport is the scoring payload returned by the brain /scoring endpoint.
type AgentScoreReport struct {
	AgentName string             `json:"agent_name"`
	Count     int                `json:"count"`
	Logs      []AgentScoreLog    `json:"logs"`
	Averages  AgentScoreAverages `json:"averages"`
}
