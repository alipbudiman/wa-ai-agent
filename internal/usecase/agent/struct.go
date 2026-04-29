package rulebase

import (
	"fmt"

	"go.mau.fi/whatsmeow/types/events"
)

const (
	ai_ability = `
	*AI Capabilities*:
- These skills can be triggered through natural conversation (no special command format needed).
- Web Search: Retrieve real-time information from the web.
- Current Time: Get the current date and time.
- Send Message: Send a WhatsApp message to a specified contact.
- Get User Information: Fetch contact details for a given WhatsApp number.
- Broadcast Message: Send a message to all contacts (one-way announcement via status story).
	- Automation Task Handler: Coordinate repeatable workflows (collect, process, and dispatch) with consistent execution flow.
	- Cronjob Scheduler Integration: Run time-based jobs for reminders, periodic checks, and scheduled notifications.
	- Multi-Step Workflow Chaining: Combine multiple API operations in sequence (lookup, enrich, summarize, then send).
	- Multi-API Interconnection: Bridge Go WhatsApp runtime and Python Brain API for modular orchestration.
	- Observability and Tracing: Track agent thinking and tool execution flow for debugging and performance tuning.
	`

	CHAT_AGENT_ROUTE     = "chat-agent-code"
	RESEARCH_AGENT_ROUTE = "agent-research"
)

var (
	capabilityDescription   string
	menuString              string
	ErrFailToProcessMessage = fmt.Errorf("Failed to process the message. Please try again later.")
)

type CommandCollection struct {
	Help                     CommandInfo
	PingPong                 CommandInfo
	About                    CommandInfo
	GetPendingMessage        CommandInfo
	SendPendingMessage       CommandInfo
	UnsendPendingMessage     CommandInfo
	GetPendingFile           CommandInfo
	SendPendingFile          CommandInfo
	UnsendPendingFile        CommandInfo
	SelectProvider           CommandInfo
	GetContactInfo           CommandInfo
	AgentResearch            CommandInfo
	AgentCode                CommandInfo
	AutoReplyAIConfig        CommandInfo
	AIGroupInteractionConfig CommandInfo
	AgentModelConfig         CommandInfo
	OllamaConfig             CommandInfo
	OpenRouterConfig         CommandInfo
	NvidiaConfig             CommandInfo
	AnthropicConfig          CommandInfo
}

type CommandAction func(*Agents, CommandArgs)

type CommandInfo struct {
	Name        string
	Description string
	Command     string
	Function    CommandAction
}

type CommandArgs struct {
	Text string
	Evt  *events.Message
}

type AgentOnHoldMessage struct {
	User          string `bson:"user"`
	PendingNumber string `bson:"pending_number"`
	Message       string `bson:"pending_message"`
}

type AgentOnHoldFile struct {
	User           string `bson:"user"`
	PendingNumber  string `bson:"pending_number"`
	PendingFile    string `bson:"pending_file"`
	PendingCaption string `bson:"pending_caption"`
}

type ContactVCardInfo struct {
	N    string
	FN   string
	WAID string
}

type InboxContent struct {
	Message     string
	ContactInfo ContactVCardInfo
}

func (i *InboxContent) IsMessage() bool {
	return i.Message != ""
}

func (i *InboxContent) IsContact() bool {
	return i.ContactInfo.WAID != ""
}

func (i *InboxContent) String() string {
	if i.IsMessage() {
		return i.Message
	}
	if i.IsContact() {
		wrapper := ";;;Contact info:\n%s\n;;;"
		info := fmt.Sprintf("\nName: %s\nFull Name: %s\nPhone: %s", i.ContactInfo.N, i.ContactInfo.FN, i.ContactInfo.WAID)
		return fmt.Sprintf(wrapper, info)
	}
	return ""
}
