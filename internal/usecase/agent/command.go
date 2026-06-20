package rulebase

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	cfg_struct "github.com/alipbudiman/wa-ai-agent.git/internal/config/struct"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"github.com/alipbudiman/wa-ai-agent.git/skilldev"
	"github.com/sashabaranov/go-openai"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func init() {
	capabilityDescription = readCapabilityDescription()
	menuString = CommandString()

	md := skilldev.NewMarkdownWriter()
	md.H1("WhatsApp AI Agent Command Menu").Text("Here are the available commands you can use to interact with the WhatsApp AI Agent:").BulletList(func() []string {
		commands := RegisteredCommands()
		var items []string
		for _, cmd := range commands.All() {
			items = append(items, fmt.Sprintf("%s - %s", cmd.Command, cmd.Description))
		}
		return items
	}())

	err := md.Save("skills/COMMAND_LISTS.md")
	if err != nil {
		panic(err)
	}
}

func readCapabilityDescription() string {
	read_skills := skilldev.ReadSkills("CAPABILITY.md")
	if read_skills == "" {
		return strings.TrimSpace(ai_ability)
	}

	return read_skills
}

func RegisteredCommands() CommandCollection {
	return CommandCollection{
		Help: CommandInfo{
			Name:        "Help",
			Description: "Displays available commands.",
			Command:     "/help",
			Function:    (*Agents).Help,
		},
		PingPong: CommandInfo{
			Name:        "Ping Pong",
			Description: "Responds with 'Pong!' to test connectivity.",
			Command:     "/ping",
			Function:    (*Agents).Ping,
		},
		About: CommandInfo{
			Name:        "About",
			Description: "Shows AI agent architecture and capability overview.",
			Command:     "/about",
			Function:    (*Agents).About,
		},
		GetPendingMessage: CommandInfo{
			Name:        "Get Pending Message",
			Description: "Retrieves the current pending on-hold message for review.",
			Command:     "/message-pending",
			Function:    (*Agents).GetPendingMessage,
		},
		SendPendingMessage: CommandInfo{
			Name:        "Send Pending Message",
			Description: "Sends a pending message on hold to the specified number.",
			Command:     "/send",
			Function:    (*Agents).SendPendingMessageFromAgent,
		},
		UnsendPendingMessage: CommandInfo{
			Name:        "Unsend Pending Message",
			Description: "Cancels and removes the current pending on-hold message.",
			Command:     "/unsend",
			Function:    (*Agents).UnsendPendingMessageFromAgent,
		},
		GetPendingFile: CommandInfo{
			Name:        "Get Pending File",
			Description: "Retrieves the current pending on-hold file for review.",
			Command:     "/file-pending",
			Function:    (*Agents).GetPendingFile,
		},
		SendPendingFile: CommandInfo{
			Name:        "Send Pending File",
			Description: "Sends a pending file on hold to the specified number.",
			Command:     "/file-send",
			Function:    (*Agents).SendPendingFileFromAgent,
		},
		UnsendPendingFile: CommandInfo{
			Name:        "Unsend Pending File",
			Description: "Cancels and removes the current pending on-hold file.",
			Command:     "/file-unsend",
			Function:    (*Agents).UnsendPendingFileFromAgent,
		},
		SelectProvider: CommandInfo{
			Name:        "Select AI Provider",
			Description: "Selects the AI provider to use for responses.",
			Command:     "/select-providers",
			Function:    (*Agents).SelectAIProvider,
		},
		GetContactInfo: CommandInfo{
			Name:        "Get Contact Info",
			Description: "Retrieves information about a specific contact.",
			Command:     "/contact-info",
			Function:    (*Agents).GetContactInfo,
		},
		AgentReply: CommandInfo{
			Name:        "Agent Reply",
			Description: "Initiate a reply query using the AI agent.",
			Command:     "/reply",
			Function:    (*Agents).ReplyAgent,
		},
		AgentResearch: CommandInfo{
			Name:        "Agent Research",
			Description: "Initiate a research query using the AI agent.",
			Command:     "/research",
			Function:    (*Agents).ResearchAgent,
		},
		AgentCode: CommandInfo{
			Name:        "Agent Code",
			Description: "Delegate task direct to Agent Code",
			Command:     "/code",
			Function:    (*Agents).AgentCode,
		},
		AutoReplyAIConfig: CommandInfo{
			Name:        "Auto-Reply AI Config",
			Description: "Configure auto-reply AI settings.",
			Command:     "/auto-reply-config",
			Function:    (*Agents).AutoReplyAIUpdateConfig,
		},
		AIGroupInteractionConfig: CommandInfo{
			Name:        "AI Group Interaction Config",
			Description: "Configure AI group interaction settings.",
			Command:     "/ai-group-interaction-config",
			Function:    (*Agents).AIGroupInteractionUpdateConfig,
		},
		AgentModelConfig: CommandInfo{
			Name:        "Agent Model Config",
			Description: "Configure the AI model used by the agent.",
			Command:     "/agent-model-config",
			Function:    (*Agents).AgentModelUpdateConfig,
		},
		OllamaConfig: CommandInfo{
			Name:        "Ollama Config",
			Description: "Configure Ollama LLM provider settings.",
			Command:     "/ollama-config",
			Function:    (*Agents).OllamaConfig,
		},
		OpenRouterConfig: CommandInfo{
			Name:        "OpenRouter Config",
			Description: "Configure OpenRouter LLM provider settings.",
			Command:     "/openrouter-config",
			Function:    (*Agents).OpenRouterConfig,
		},
		NvidiaConfig: CommandInfo{
			Name:        "Nvidia Config",
			Description: "Configure Nvidia LLM provider settings.",
			Command:     "/nvidia-config",
			Function:    (*Agents).NvidiaConfig,
		},
		AnthropicConfig: CommandInfo{
			Name:        "Anthropic Config",
			Description: "Configure Anthropic LLM provider settings.",
			Command:     "/anthropic-config",
			Function:    (*Agents).AnthropicConfig,
		},
		GoogleConfig: CommandInfo{
			Name:        "Google Config",
			Description: "Configure Google LLM provider settings.",
			Command:     "/google-config",
			Function:    (*Agents).GoogleConfig,
		},
		MinimaxConfig: CommandInfo{
			Name:        "Minimax Config",
			Description: "Configure Minimax LLM provider settings.",
			Command:     "/minimax-config",
			Function:    (*Agents).MinimaxConfig,
		},
		OpenAIConfig: CommandInfo{
			Name:        "OpenAI Config",
			Description: "Configure OpenAI LLM provider settings.",
			Command:     "/openai-config",
			Function:    (*Agents).OpenAIConfig,
		},
		TavilyConfig: CommandInfo{
			Name:        "Tavily Config",
			Description: "Configure Tavily web search settings.",
			Command:     "/tavily-config",
			Function:    (*Agents).TavilyConfig,
		},
		LangSmithConfig: CommandInfo{
			Name:        "LangSmith Config",
			Description: "Configure LangSmith tracing settings.",
			Command:     "/langsmith-config",
			Function:    (*Agents).LangSmithConfig,
		},
		GinServerConfig: CommandInfo{
			Name:        "Gin Server Config",
			Description: "Configure Gin server integration settings.",
			Command:     "/gin-server-config",
			Function:    (*Agents).GinServerConfig,
		},
		ShowAllContacts: CommandInfo{
			Name:        "Show All Contacts",
			Description: "Displays a list of all contacts.",
			Command:     "/show-contacts",
			Function:    (*Agents).ShowAllContacts,
		},
	}
}

func (c CommandCollection) All() []CommandInfo {
	return []CommandInfo{
		c.Help,
		c.PingPong,
		c.About,
		c.GetPendingMessage,
		c.SendPendingMessage,
		c.UnsendPendingMessage,
		c.GetPendingFile,
		c.SendPendingFile,
		c.UnsendPendingFile,
		c.SelectProvider,
		c.GetContactInfo,
		c.AgentCode,
		c.AgentResearch,
		c.AgentReply,
		c.AutoReplyAIConfig,
		c.AIGroupInteractionConfig,
		c.AgentModelConfig,
		c.OllamaConfig,
		c.OpenRouterConfig,
		c.NvidiaConfig,
		c.AnthropicConfig,
		c.GoogleConfig,
		c.MinimaxConfig,
		c.OpenAIConfig,
		c.TavilyConfig,
		c.LangSmithConfig,
		c.GinServerConfig,
		c.ShowAllContacts,
	}
}

func CommandString() string {
	commands := RegisteredCommands()
	var menuText strings.Builder
	menuText.WriteString("Available Commands:\n")
	for _, cmd := range commands.All() {
		menuText.WriteString(cmd.Command + " - " + cmd.Description + "\n")
	}
	menuText.WriteString("\nUse the command followed by any required parameters to execute.")
	menuText.WriteString("\nExample: /ping")
	return menuText.String()
}

func (h *Agents) Help(args CommandArgs) {
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(menuString),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}

}

func (h *Agents) Ping(args CommandArgs) {
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Pong!"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}
}

func (h *Agents) About(args CommandArgs) {
	var aboutText strings.Builder
	aboutText.WriteString("I am a WhatsApp AI agent created by Alif Budiman for a thesis project.\n\n")
	aboutText.WriteString("System highlights:\n")
	aboutText.WriteString("- Integrated LLM-based reasoning and response generation.\n")
	aboutText.WriteString("- LangChain/LangGraph agent-flow mechanism for routing, tools, and memory-aware execution.\n")
	aboutText.WriteString("- Multi-API handlers for WhatsApp operations (send message, contact lookup, help menu, broadcast).\n")
	aboutText.WriteString("- Interconnection architecture between Go WhatsApp runtime and Python Brain API for orchestration.\n")
	aboutText.WriteString("- Automation task handler pipelines for repeatable agent workflows.\n")
	aboutText.WriteString("- Cronjob-ready scheduling layer for periodic reminders/checks/notifications.\n")
	aboutText.WriteString("- Trace-friendly workflow observability for debugging and optimization.\n")
	aboutText.WriteString("- Automation Task Handler: Coordinate repeatable workflows (collect, process, and dispatch) with consistent execution flow.\n")
	aboutText.WriteString("- Cronjob Scheduler Integration: Run time-based jobs for reminders, periodic checks, and scheduled notifications.\n")
	aboutText.WriteString("- Multi-API Interconnection: Bridge Go WhatsApp runtime and Python Brain API for modular orchestration.\n")
	aboutText.WriteString("- Multi-Step Workflow Chaining: Combine multiple API operations in sequence (lookup, enrich, summarize, then send).\n")
	aboutText.WriteString("- Observability and Tracing: Track agent thinking and tool execution flow for debugging and performance tuning.\n")

	if capability := strings.TrimSpace(capabilityDescription); capability != "" {
		aboutText.WriteString("\nCapability Snapshot:\n")
		aboutText.WriteString(capability)
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(aboutText.String()),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) GetPendingMessage(args CommandArgs) {
	var pending_msg AgentOnHoldMessage
	filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_message"}
	err := databases.MongoManager.FindOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		"tmp_messages",
		filter,
		&pending_msg,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("No pending messages found."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(fmt.Sprintf("Pending message to send to %s:\n\n```\n%s```", pending_msg.PendingNumber, pending_msg.Message)),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) SendPendingMessageFromAgent(args CommandArgs) {
	proc, err := h.throwToAgent(args, RegisteredCommands().SendPendingMessage.Command)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to process command: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	if proc {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Processing your request to send message..."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	var pending_msg AgentOnHoldMessage
	filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_message"}
	err = databases.MongoManager.FindOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		"tmp_messages",
		filter,
		&pending_msg,
	)

	if err != nil {
		if err == databases.ErrDocumentNotFound {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("No pending messages found."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	new_jid, err := h.waclient.ParseJID(pending_msg.PendingNumber)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Invalid pending number format: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	err = h.waclient.SendText(context.Background(), new_jid.String(), &waE2E.Message{
		Conversation: proto.String(pending_msg.Message),
	})
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	// Clear the pending message after sending
	delete_filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_message"}
	results, err := databases.MongoManager.DeleteOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		"tmp_messages",
		delete_filter,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to clear pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if results.DeletedCount == 0 {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Pending message sent but failed to clear from database."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	name := pending_msg.PendingNumber

	contact_info, err := h.waclient.GetContact(context.Background(), pending_msg.PendingNumber)
	if err != nil {
		log.Printf("error getting contact info for pending number %s: %v", pending_msg.PendingNumber, err)
	}

	name = contact_info.PushName
	if contact_info.FullName != "" {
		name = contact_info.FullName
	}

	err = h.manager.StoreInboxMessage(ports.InboxMessage{
		JID:        new_jid,
		MsgID:      pkg.GenerateUID(),
		FullName:   contact_info.FullName,
		PushName:   contact_info.PushName,
		Number:     pending_msg.PendingNumber,
		Message:    pending_msg.Message,
		FromMe:     true,
		ReceivedAt: time.Now().Unix(),
		Read:       false,
	})
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(err.Error()),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
	}

	senderHistoryMessage := fmt.Sprintf(
		"SYSTEM EVENT: The pending WhatsApp message to %s (%s) has been confirmed with /send and sent successfully.\nSent message:\n%s",
		name,
		pending_msg.PendingNumber,
		pending_msg.Message,
	)

	resolvedSender := h.resolveSender(args.Evt)
	recipientHistoryMessage := fmt.Sprintf(
		"SYSTEM EVENT: The AI agent sent this WhatsApp message to this thread on behalf of %s.\nSent message:\n%s",
		resolvedSender.String(),
		pending_msg.Message,
	)

	senderReplyStatus, err := h.recordAgentHistory(
		context.Background(),
		resolvedSender,
		h.isFromGroup(args.Evt),
		senderHistoryMessage,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Message was sent, but failed to notify sender agent history: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	recipientReplyStatus, err := h.recordAgentHistory(
		context.Background(),
		new_jid,
		recipientThreadGroupID(new_jid),
		recipientHistoryMessage,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Message was sent, but failed to notify recipient agent history: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if senderReplyStatus == ai_providers_api.SUCCESS && recipientReplyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ Message has been sent by the agent"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	} else {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Message was sent, but failed to update all agent histories."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}

}

func (h *Agents) UnsendPendingMessageFromAgent(args CommandArgs) {
	var pending_msg AgentOnHoldMessage
	filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_message"}
	err := databases.MongoManager.FindOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		h.config.App.Manager.TemplateMsgCollectionName,
		filter,
		&pending_msg,
	)

	if err != nil {
		if err == databases.ErrDocumentNotFound {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("No pending messages found."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	deleteFilter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_message"}
	results, err := databases.MongoManager.DeleteOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		h.config.App.Manager.TemplateMsgCollectionName,
		deleteFilter,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to cancel pending message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if results.DeletedCount == 0 {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("No pending messages found."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	name := pending_msg.PendingNumber
	contact_info, err := h.waclient.GetContact(context.Background(), pending_msg.PendingNumber)
	if err != nil {
		log.Printf("error getting contact info for pending number %s: %v", pending_msg.PendingNumber, err)
	}

	name = contact_info.PushName
	if contact_info.FullName != "" {
		name = contact_info.FullName
	}

	resolvedSender := h.resolveSender(args.Evt)
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: fmt.Sprintf("Send message to %s has been canceled.", name),
			},
		},
		true,
		ports.HISTORY,
		resolvedSender,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send cancellation confirmation: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ Tell Agent to Unsend Pending Message"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}
}

func (h *Agents) GetPendingFile(args CommandArgs) {
	var pending_file AgentOnHoldFile
	filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_file"}
	err := databases.MongoManager.FindOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		"tmp_messages",
		filter,
		&pending_file,
	)
	if err != nil {
		if err == databases.ErrDocumentNotFound {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("No pending files found."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving pending file: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(fmt.Sprintf("Pending file to send to %s:\n\nFile: %s\nCaption: %s", pending_file.PendingNumber, pending_file.PendingFile, pending_file.PendingCaption)),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) SendPendingFileFromAgent(args CommandArgs) {

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("⚙️ Processing your request to send file..."),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}

	go func() {
		proc, err := h.throwToAgent(args, RegisteredCommands().SendPendingFile.Command)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to process command: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if proc {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Processing your request to send file..."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		var pending_file AgentOnHoldFile
		filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_file"}
		err = databases.MongoManager.FindOne(
			h.config.Databases.MongoDB.DBAlias,
			h.config.Databases.MongoDB.DBName,
			"tmp_messages",
			filter,
			&pending_file,
		)

		if err != nil {
			if err == databases.ErrDocumentNotFound {
				if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
					Conversation: proto.String("No pending files found."),
				}); err != nil {
					log.Printf("failed to send reply message: %v", err)
				}
				return
			}
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Error retrieving pending file: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		new_jid, err := h.waclient.ParseJID(pending_file.PendingNumber)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Invalid pending number format: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		caption := pending_file.PendingCaption
		if caption == "" {
			caption = "File attached"
		}

		err = h.waclient.SendFile(context.Background(), new_jid.String(), pending_file.PendingFile, caption)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to send pending file: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if err := os.Remove(pending_file.PendingFile); err != nil {
			log.Printf("failed to delete file: %v", err)
		}

		// Clear the pending file after sending
		delete_filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_file"}
		results, err := databases.MongoManager.DeleteOne(
			h.config.Databases.MongoDB.DBAlias,
			h.config.Databases.MongoDB.DBName,
			"tmp_messages",
			delete_filter,
		)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to clear pending file: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if results.DeletedCount == 0 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Pending file sent but failed to clear from database."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		name := pending_file.PendingNumber

		contact_info, err := h.waclient.GetContact(context.Background(), pending_file.PendingNumber)
		if err != nil {
			log.Printf("error getting contact info for pending number %s: %v", pending_file.PendingNumber, err)
		}

		name = contact_info.PushName
		if contact_info.FullName != "" {
			name = contact_info.FullName
		}

		err = h.manager.StoreInboxMessage(ports.InboxMessage{
			JID:        new_jid,
			MsgID:      pkg.GenerateUID(),
			FullName:   contact_info.FullName,
			PushName:   contact_info.PushName,
			Number:     pending_file.PendingNumber,
			Message:    fmt.Sprintf("[File Attached] %s", pending_file.PendingCaption),
			FromMe:     true,
			ReceivedAt: time.Now().Unix(),
			Read:       false,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(err.Error()),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
		}

		senderHistoryMessage := fmt.Sprintf(
			"SYSTEM EVENT: The pending WhatsApp file to %s (%s) has been confirmed with /file-send and sent successfully.\nFile: %s\nCaption: %s",
			name,
			pending_file.PendingNumber,
			pending_file.PendingFile,
			pending_file.PendingCaption,
		)

		resolvedSender := h.resolveSender(args.Evt)
		recipientHistoryMessage := fmt.Sprintf(
			"SYSTEM EVENT: The AI agent sent a WhatsApp file to this thread on behalf of %s.\nFile: %s\nCaption: %s",
			resolvedSender.String(),
			pending_file.PendingFile,
			pending_file.PendingCaption,
		)

		senderReplyStatus, err := h.recordAgentHistory(
			context.Background(),
			resolvedSender,
			h.isFromGroup(args.Evt),
			senderHistoryMessage,
		)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("File was sent, but failed to notify sender agent history: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
				return
			}
			return
		}

		recipientReplyStatus, err := h.recordAgentHistory(
			context.Background(),
			new_jid,
			recipientThreadGroupID(new_jid),
			recipientHistoryMessage,
		)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("File was sent, but failed to notify recipient agent history: %v", err)),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
				return
			}
			return
		}

		if senderReplyStatus == ai_providers_api.SUCCESS && recipientReplyStatus == ai_providers_api.SUCCESS {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("⚙️ File has been sent by the agent"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
				return
			}
		} else {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("File was sent, but failed to update all agent histories."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
				return
			}
		}
	}()
}

func (h *Agents) UnsendPendingFileFromAgent(args CommandArgs) {
	var pending_file AgentOnHoldFile
	filter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_file"}
	err := databases.MongoManager.FindOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		h.config.App.Manager.TemplateMsgCollectionName,
		filter,
		&pending_file,
	)

	if err != nil {
		if err == databases.ErrDocumentNotFound {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("No pending files found."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving pending file: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := os.Remove(pending_file.PendingFile); err != nil {
		log.Printf("failed to delete file: %v", err)
	}

	deleteFilter := bson.M{"user": args.Evt.Info.Chat.String(), "parameter": "send_file"}
	results, err := databases.MongoManager.DeleteOne(
		h.config.Databases.MongoDB.DBAlias,
		h.config.Databases.MongoDB.DBName,
		h.config.App.Manager.TemplateMsgCollectionName,
		deleteFilter,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to cancel pending file: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if results.DeletedCount == 0 {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("No pending files found."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	name := pending_file.PendingNumber
	contact_info, err := h.waclient.GetContact(context.Background(), pending_file.PendingNumber)
	if err != nil {
		log.Printf("error getting contact info for pending number %s: %v", pending_file.PendingNumber, err)
	}

	name = contact_info.PushName
	if contact_info.FullName != "" {
		name = contact_info.FullName
	}

	resolvedSender := h.resolveSender(args.Evt)
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: fmt.Sprintf("Send file to %s has been canceled.", name),
			},
		},
		true,
		ports.HISTORY,
		resolvedSender,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send cancellation confirmation: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ Tell Agent to Unsend Pending File"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}
}

func (h *Agents) SelectAIProvider(args CommandArgs) {
	cmd := RegisteredCommands().SelectProvider
	selected := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))
	list_providers, err := h.chatCompleter.GetAIDataStatus(context.Background())
	if err != nil {
		list_providers = &ports.AIDataStatus{
			ListAIProviders: []string{"ollama", "openrouter", "nvidia", "anthropic", "google"},
		}
		log.Printf("failed to get AI providers: %v", err)
	}

	provider_selected, err := h.manager.GetSelectedAIProvider()
	if err != nil {
		provider_selected = "not set"
	}

	if selected == "" {
		var builder strings.Builder
		if provider_selected != "not set" {
			fmt.Fprintf(&builder, "Current selected AI provider: '%s'.\nSelect a new provider from the list below:\n", provider_selected)
		} else {
			builder.WriteString("Please select an AI provider from the following list:\n")
		}
		for _, provider := range list_providers.ListAIProviders {
			builder.WriteString("- " + provider + "\n")
		}
		builder.WriteString("\nUsage: " + cmd.Command + " [provider_name]")
		builder.WriteString("\nExample: " + cmd.Command + " " + list_providers.ListAIProviders[0])
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(builder.String()),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if !slices.Contains(list_providers.ListAIProviders, selected) {
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("Invalid provider '%s'. Please select from the following list:\n", selected))
		for _, provider := range list_providers.ListAIProviders {
			builder.WriteString("- " + provider + "\n")
		}
		builder.WriteString("\nUsage: " + cmd.Command + " [provider_name]")
		builder.WriteString("\nExample: " + cmd.Command + " " + list_providers.ListAIProviders[0])
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(builder.String()),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	err = h.manager.SetSelectedAIProvider(selected)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to set selected AI provider: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(fmt.Sprintf("Selected AI provider has been set to '%s'.", selected)),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}
}

func (h *Agents) GetContactInfo(args CommandArgs) {
	cmd := RegisteredCommands().GetContactInfo
	number := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))
	if number == "" {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Please provide a phone number to look up. Usage: " + cmd.Command + " [phone_number]"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}
	contact_info, err := h.waclient.GetContact(context.Background(), number)
	if err != nil {
		log.Printf("error getting contact info: %v", err)
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Error retrieving contact info: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	infoText := fmt.Sprintf("Contact Info:\nName: %s\nPush Name: %s",
		contact_info.FullName, contact_info.PushName)

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(infoText),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) ReplyAgent(args CommandArgs) {
	cmd := RegisteredCommands().AgentReply
	prompt := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if prompt == "" {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Please provide a prompt to reply to the agent. Usage: " + cmd.Command + " [prompt]"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	resolvedSender := h.resolveSender(args.Evt)
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: prompt,
			},
		},
		false,
		ports.HISTORY,
		resolvedSender,
		REPLY_AGENT_ROUTE,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send reply to agent: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("📨 Your message has been sent to the agent."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	} else {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Failed to notify agent about your reply."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}
}

func (h *Agents) ResearchAgent(args CommandArgs) {
	cmd := RegisteredCommands().AgentResearch
	prompt := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if prompt == "" {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Please provide a prompt to perform research. Usage: " + cmd.Command + " [prompt]"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	resolvedSender := h.resolveSender(args.Evt)
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: prompt,
			},
		},
		true,
		ports.HISTORY,
		resolvedSender,
		RESEARCH_AGENT_ROUTE,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to perform research: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("🔎 Research Agent is processing your request... Please wait."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}

}

func (h *Agents) AgentCode(args CommandArgs) {
	cmd := RegisteredCommands().AgentCode
	prompt := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if prompt == "" {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Please provide a prompt to generate code. Usage: " + cmd.Command + " [prompt]"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	resolvedSender := h.resolveSender(args.Evt)
	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: prompt,
			},
		},
		true,
		ports.HISTORY,
		resolvedSender,
		CHAT_AGENT_ROUTE,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to generate code: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ Agent Process... Please wait while the agent processes your request."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	} else {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Failed to notify agent about code generation."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}

}

func (h *Agents) OllamaConfig(args CommandArgs) {
	cmd := RegisteredCommands().OllamaConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		ollama_config_info := cfg_struct.GetOllamaConfig()
		config_help := "Ollama Config Command Usage:\n" +
			"To set host: `/ollama-config host [hostname]`\n" +
			"To set model: `/ollama-config model [model_name]`\n" +
			"Example: `/ollama-config host http://localhost:11434`\n" +
			"\n*See Ollama models on*\n https://ollama.com/search" +
			"\n\n*Current Ollama Config:*\n" +
			fmt.Sprintf("- Host: %s\n", ollama_config_info.BaseURL) +
			fmt.Sprintf("- Model: %s\n", ollama_config_info.Model)

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "host "); ok {
		host := strings.TrimSpace(after)
		err := cfg_struct.UpdateOllamaConfig(cfg_struct.OllamaConfig{BaseURL: host})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Ollama host. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Ollama host set to '%s'.", host)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		model := strings.TrimSpace(after)
		err := cfg_struct.UpdateOllamaConfig(cfg_struct.OllamaConfig{Model: model})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Ollama model. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Ollama model set to '%s'.", model)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set host: `/ollama-config host [hostname]`\n" +
			"To set model: `/ollama-config model [model_name]`\n" +
			"Example: `/ollama-config host http://localhost:11434`\n" +
			"\n*See Ollama models on*\n https://ollama.com/search"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) OpenRouterConfig(args CommandArgs) {
	cmd := RegisteredCommands().OpenRouterConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		openrouter_config_info := cfg_struct.GetOpenRouterConfig()
		config_help := "OpenRouter Config Command Usage:\n" +
			"To set API key: `/openrouter-config api-key [your_api_key]`\n" +
			"To delete API key: `/openrouter-config api-key delete`\n" +
			"To set model: `/openrouter-config model [model_name]`\n" +
			"Example: `/openrouter-config api-key sk-xxxxxx`\n" +
			"\n*See OpenRouter API documentation on*\n https://openrouter.ai/models" +
			"\n*Current OpenRouter Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(openrouter_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Model: %s\n", openrouter_config_info.Model)

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		apiKey := strings.TrimSpace(after)
		if strings.ToLower(apiKey) == "delete" {
			apiKey = "-DELETE-"
		}
		err := cfg_struct.UpdateOpenRouterConfig(cfg_struct.OpenRouterConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set OpenRouter API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "OpenRouter API key has been updated."
		if apiKey == "-DELETE-" {
			successMsg = "OpenRouter API key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		model := strings.TrimSpace(after)
		err := cfg_struct.UpdateOpenRouterConfig(cfg_struct.OpenRouterConfig{Model: model})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set OpenRouter model. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("OpenRouter model has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set OpenRouter API key: `/openrouter-config api-key [your_api_key]`\n" +
			"To delete API key: `/openrouter-config api-key delete`\n" +
			"Example: `/openrouter-config api-key sk-xxxxxx`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) NvidiaConfig(args CommandArgs) {
	cmd := RegisteredCommands().NvidiaConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		nvidia_config_info := cfg_struct.GetNvidiaConfig()
		config_help := "NVIDIA Config Command Usage:\n" +
			"To set API key: `/nvidia-config api-key [your_api_key]`\n" +
			"To delete API key: `/nvidia-config api-key delete`\n" +
			"To set model: `/nvidia-config model [model_name]`\n" +
			"Example: `/nvidia-config api-key sk-xxxxxx`\n" +
			"\n*See NVIDIA API models on*\n https://build.nvidia.com/models" +
			"\n*Current NVIDIA Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(nvidia_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Model: %s\n", nvidia_config_info.Model)

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		apiKey := strings.TrimSpace(after)
		if strings.ToLower(apiKey) == "delete" {
			apiKey = "-DELETE-"
		}
		err := cfg_struct.UpdateNvidiaConfig(cfg_struct.NvidiaConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set NVIDIA API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "NVIDIA API key has been updated."
		if apiKey == "-DELETE-" {
			successMsg = "NVIDIA API key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		model := strings.TrimSpace(after)
		err := cfg_struct.UpdateNvidiaConfig(cfg_struct.NvidiaConfig{Model: model})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set NVIDIA model. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("NVIDIA model has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set NVIDIA API key: `/nvidia-config api-key [your_api_key]`\n" +
			"To delete API key: `/nvidia-config api-key delete`\n" +
			"Example: `/nvidia-config api-key sk-xxxxxx`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) AnthropicConfig(args CommandArgs) {
	cmd := RegisteredCommands().AnthropicConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		anthropic_config_info := cfg_struct.GetAnthropicConfig()
		config_help := "Anthropic Config Command Usage:\n" +
			"To set API key: `/anthropic-config api-key [your_api_key]`\n" +
			"To delete API key: `/anthropic-config api-key delete`\n" +
			"To set model: `/anthropic-config model [model_name]`\n" +
			"To set Base URL: `/anthropic-config base-url [url]`\n" +
			"To set AuthToken: `/anthropic-config auth-token [token]`\n" +
			"To delete AuthToken: `/anthropic-config auth-token delete`\n" +
			"Example: `/anthropic-config api-key sk-xxxxxx`\n" +
			"\n*See Anthropic API documentation on*\n https://platform.claude.com/docs/en/about-claude/models/overview" +
			"\n*Current Anthropic Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(anthropic_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Model: %s\n", anthropic_config_info.Model) +
			fmt.Sprintf("- Base URL: %s\n", anthropic_config_info.BaseURL) +
			fmt.Sprintf("- Auth Token: %s\n", pkg.MaskString(anthropic_config_info.AuthToken, 4, 4))

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		apiKey := strings.TrimSpace(after)
		if strings.ToLower(apiKey) == "delete" {
			apiKey = "-DELETE-"
		}
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Anthropic API key has been updated."
		if apiKey == "-DELETE-" {
			successMsg = "Anthropic API key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		model := strings.TrimSpace(after)
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{Model: model})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic model. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Anthropic model has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "base-url "); ok {
		baseURL := strings.TrimSpace(after)
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{BaseURL: baseURL})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic Base URL. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Anthropic Base URL has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "auth-token "); ok {
		authToken := strings.TrimSpace(after)
		if strings.ToLower(authToken) == "delete" {
			authToken = "-DELETE-"
		}
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{AuthToken: authToken})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic Auth Token. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Anthropic Auth Token has been updated."
		if authToken == "-DELETE-" {
			successMsg = "Anthropic Auth Token has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set Anthropic API key: `/anthropic-config api-key [your_api_key]`\n" +
			"To delete API key: `/anthropic-config api-key delete`\n" +
			"To delete AuthToken: `/anthropic-config auth-token delete`\n" +
			"Example: `/anthropic-config api-key sk-xxxxxx`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) GoogleConfig(args CommandArgs) {
	cmd := RegisteredCommands().GoogleConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		google_config_info := cfg_struct.GetGoogleConfig()
		config_help := "Google Config Command Usage:\n" +
			"To set API Key: `/google-config api-key [api_key]`\n" +
			"To delete API Key: `/google-config api-key delete`\n" +
			"To set Model: `/google-config model [model]`\n" +
			"To set Image Model: `/google-config img-model [img_model]`\n" +
			"Example: `/google-config model gemini-2.5-flash`\n\n" +
			"*Current Google Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(google_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Model: %s\n", google_config_info.Model) +
			fmt.Sprintf("- Image Model: %s\n", google_config_info.ImgModel)
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/google-config api-key [api_key]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		api_key := configArgs[0]
		if strings.ToLower(api_key) == "delete" {
			api_key = "-DELETE-"
		}
		err := cfg_struct.UpdateGoogleConfig(cfg_struct.GoogleConfig{
			APIKey: api_key,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update google config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Google config has been updated."
		if api_key == "-DELETE-" {
			successMsg = "Google API Key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/google-config model [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		model := configArgs[0]
		err := cfg_struct.UpdateGoogleConfig(cfg_struct.GoogleConfig{
			Model: model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update google config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Google config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "img-model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/google-config img-model [img_model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		img_model := configArgs[0]
		err := cfg_struct.UpdateGoogleConfig(cfg_struct.GoogleConfig{
			ImgModel: img_model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update google config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Google config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid google config command. Usage:\n" +
			"To set API Key: `/google-config api-key [api_key]`\n" +
			"To delete API Key: `/google-config api-key delete`\n" +
			"To set Model: `/google-config model [model]`\n" +
			"To set Image Model: `/google-config img-model [img_model]`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) MinimaxConfig(args CommandArgs) {
	cmd := RegisteredCommands().MinimaxConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		minimax_config_info := cfg_struct.GetMinimaxConfig()
		config_help := "Minimax Config Command Usage:\n" +
			"To set API Key: `/minimax-config api-key [api_key]`\n" +
			"To delete API Key: `/minimax-config api-key delete`\n" +
			"To set Group ID: `/minimax-config group-id [group_id]`\n" +
			"To delete Group ID: `/minimax-config group-id delete`\n" +
			"To set Model: `/minimax-config model [model]`\n" +
			"To set Image Model: `/minimax-config img-model [img_model]`\n" +
			"Example: `/minimax-config model image-01`\n\n" +
			"*Current Minimax Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(minimax_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Group ID: %s\n", minimax_config_info.GroupID) +
			fmt.Sprintf("- Model: %s\n", minimax_config_info.Model) +
			fmt.Sprintf("- Image Model: %s\n", minimax_config_info.ImgModel)
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/minimax-config api-key [api_key]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		api_key := configArgs[0]
		if strings.ToLower(api_key) == "delete" {
			api_key = "-DELETE-"
		}
		err := cfg_struct.UpdateMinimaxConfig(cfg_struct.MinimaxConfig{
			APIKey: api_key,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update minimax config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Minimax config has been updated."
		if api_key == "-DELETE-" {
			successMsg = "Minimax API Key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "group-id "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/minimax-config group-id [group_id]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		group_id := configArgs[0]
		if strings.ToLower(group_id) == "delete" {
			group_id = "-DELETE-"
		}
		err := cfg_struct.UpdateMinimaxConfig(cfg_struct.MinimaxConfig{
			GroupID: group_id,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update minimax config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Minimax Group ID has been updated."
		if group_id == "-DELETE-" {
			successMsg = "Minimax Group ID has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/minimax-config model [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		model := configArgs[0]
		err := cfg_struct.UpdateMinimaxConfig(cfg_struct.MinimaxConfig{
			Model: model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update minimax config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Minimax config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "img-model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/minimax-config img-model [img_model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		img_model := configArgs[0]
		err := cfg_struct.UpdateMinimaxConfig(cfg_struct.MinimaxConfig{
			ImgModel: img_model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update minimax config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Minimax config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid minimax config command. Usage:\n" +
			"To set API Key: `/minimax-config api-key [api_key]`\n" +
			"To delete API Key: `/minimax-config api-key delete`\n" +
			"To set Model: `/minimax-config model [model]`\n" +
			"To set Image Model: `/minimax-config img-model [img_model]`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) OpenAIConfig(args CommandArgs) {
	cmd := RegisteredCommands().OpenAIConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		openai_config_info := cfg_struct.GetOpenAIConfig()
		config_help := "OpenAI Config Command Usage:\n" +
			"To set API Key: `/openai-config api-key [api_key]`\n" +
			"To delete API Key: `/openai-config api-key delete`\n" +
			"To set Model: `/openai-config model [model]`\n" +
			"To set Image Model: `/openai-config img-model [img_model]`\n" +
			"To set Image Quality: `/openai-config img-quality [low|medium|high|auto]`\n" +
			"Example: `/openai-config model gpt-5`\n\n" +
			"*Current OpenAI Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(openai_config_info.APIKey, 4, 4)) +
			fmt.Sprintf("- Model: %s\n", openai_config_info.Model) +
			fmt.Sprintf("- Image Model: %s\n", openai_config_info.ImgModel) +
			fmt.Sprintf("- Image Quality: %s\n", openai_config_info.ImgQuality)
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/openai-config api-key [api_key]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		api_key := configArgs[0]
		if strings.ToLower(api_key) == "delete" {
			api_key = "-DELETE-"
		}
		err := cfg_struct.UpdateOpenAIConfig(cfg_struct.OpenAIConfig{
			APIKey: api_key,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update OpenAI config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "OpenAI config has been updated."
		if api_key == "-DELETE-" {
			successMsg = "OpenAI API Key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/openai-config model [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		model := configArgs[0]
		err := cfg_struct.UpdateOpenAIConfig(cfg_struct.OpenAIConfig{
			Model: model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update OpenAI config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("OpenAI config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "img-model "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/openai-config img-model [img_model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		img_model := configArgs[0]
		err := cfg_struct.UpdateOpenAIConfig(cfg_struct.OpenAIConfig{
			ImgModel: img_model,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update OpenAI config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("OpenAI config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "img-quality "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/openai-config img-quality [low|medium|high|auto]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		img_quality := strings.ToLower(configArgs[0])
		switch img_quality {
		case "low", "medium", "high", "auto":
		default:
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid image quality. Use one of: low, medium, high, auto."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		err := cfg_struct.UpdateOpenAIConfig(cfg_struct.OpenAIConfig{
			ImgQuality: img_quality,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update OpenAI config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("OpenAI config has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid OpenAI config command. Usage:\n" +
			"To set API Key: `/openai-config api-key [api_key]`\n" +
			"To delete API Key: `/openai-config api-key delete`\n" +
			"To set Model: `/openai-config model [model]`\n" +
			"To set Image Model: `/openai-config img-model [img_model]`\n" +
			"To set Image Quality: `/openai-config img-quality [low|medium|high|auto]`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) TavilyConfig(args CommandArgs) {
	cmd := RegisteredCommands().TavilyConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		tavilyConfigInfo := cfg_struct.GetTavilyConfig()
		configHelp := "Tavily Config Command Usage:\n" +
			"To set API Key: `/tavily-config api-key [api_key]`\n" +
			"To delete API Key: `/tavily-config api-key delete`\n" +
			"Example: `/tavily-config api-key tvly-xxxxxx`\n\n" +
			"*Current Tavily Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(tavilyConfigInfo.APIKey, 4, 4))

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(configHelp),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/tavily-config api-key [api_key]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		apiKey := configArgs[0]
		if strings.ToLower(apiKey) == "delete" {
			apiKey = "-DELETE-"
		}

		err := cfg_struct.UpdateTavilyConfig(cfg_struct.TavilyConfigDB{
			APIKey: apiKey,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update Tavily config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "Tavily config has been updated."
		if apiKey == "-DELETE-" {
			successMsg = "Tavily API Key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid Tavily config command. Usage:\n" +
			"To set API Key: `/tavily-config api-key [api_key]`\n" +
			"To delete API Key: `/tavily-config api-key delete`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) LangSmithConfig(args CommandArgs) {
	cmd := RegisteredCommands().LangSmithConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		langSmithConfigInfo := cfg_struct.LangSmithConfig{}.GetLangSmithConfig()
		configHelp := "LangSmith Config Command Usage:\n" +
			"To set API Key: `/langsmith-config api-key [api_key]`\n" +
			"To delete API Key: `/langsmith-config api-key delete`\n" +
			"To set project name: `/langsmith-config project-name [project_name]`\n" +
			"Example: `/langsmith-config project-name wa_bot_ai_trace`\n\n" +
			"*Current LangSmith Config:*\n" +
			fmt.Sprintf("- API Key: %s\n", pkg.MaskString(langSmithConfigInfo.APIKey(), 4, 4)) +
			fmt.Sprintf("- Project Name: %s\n", langSmithConfigInfo.ProjectName())

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(configHelp),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "api-key "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 1 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/langsmith-config api-key [api_key]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		apiKey := configArgs[0]
		if strings.ToLower(apiKey) == "delete" {
			apiKey = "-DELETE-"
		}

		err := cfg_struct.UpdateLangSmithConfig(cfg_struct.LangSmithConfigDB{
			APIKey: apiKey,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update LangSmith config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		successMsg := "LangSmith config has been updated."
		if apiKey == "-DELETE-" {
			successMsg = "LangSmith API Key has been deleted."
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(successMsg),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "project-name "); ok {
		projectName := strings.TrimSpace(after)
		if projectName == "" {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/langsmith-config project-name [project_name]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		err := cfg_struct.UpdateLangSmithConfig(cfg_struct.LangSmithConfigDB{
			ProjectName: projectName,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update LangSmith config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("LangSmith project name has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid LangSmith config command. Usage:\n" +
			"To set API Key: `/langsmith-config api-key [api_key]`\n" +
			"To delete API Key: `/langsmith-config api-key delete`\n" +
			"To set project name: `/langsmith-config project-name [project_name]`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) GinServerConfig(args CommandArgs) {
	cmd := RegisteredCommands().GinServerConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		ginConfigInfo := cfg_struct.GinServerConfig{}.GetGinConfig()
		configHelp := "Gin Server Config Command Usage:\n" +
			"To set host: `/gin-server-config host [host]`\n" +
			"To set port: `/gin-server-config port [port]`\n" +
			"To set app mode: `/gin-server-config app-mode [debug|release|test]`\n" +
			"Example: `/gin-server-config host localhost`\n\n" +
			"*Current Gin Server Config:*\n" +
			fmt.Sprintf("- Host: %s\n", ginConfigInfo.Host) +
			fmt.Sprintf("- Port: %s\n", ginConfigInfo.Port) +
			fmt.Sprintf("- App Mode: %s\n", ginConfigInfo.AppMod)

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(configHelp),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "host "); ok {
		host := strings.TrimSpace(after)
		if host == "" {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/gin-server-config host [host]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		err := cfg_struct.UpdateGinServerConfig(cfg_struct.GinServerConfig{
			Host: host,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update Gin server config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Gin server host has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "port "); ok {
		port := strings.TrimSpace(after)
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber <= 0 || portNumber > 65535 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid port. Use a number between 1 and 65535."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		err = cfg_struct.UpdateGinServerConfig(cfg_struct.GinServerConfig{
			Port: port,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update Gin server config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Gin server port has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "app-mode "); ok {
		appMode := cfg_struct.GinMod(strings.ToLower(strings.TrimSpace(after)))
		switch appMode {
		case cfg_struct.DebugMode, cfg_struct.ReleaseMode, cfg_struct.TestMode:
		default:
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid app mode. Use one of: debug, release, test."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		err := cfg_struct.UpdateGinServerConfig(cfg_struct.GinServerConfig{
			AppMod: appMode,
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update Gin server config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Gin server app mode has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid Gin server config command. Usage:\n" +
			"To set host: `/gin-server-config host [host]`\n" +
			"To set port: `/gin-server-config port [port]`\n" +
			"To set app mode: `/gin-server-config app-mode [debug|release|test]`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) AutoReplyAIUpdateConfig(args CommandArgs) {
	cmd := RegisteredCommands().AutoReplyAIConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		auto_reply_stat := h.config.App.AIReply.Enable()
		config_help := "Auto-Reply AI Config Command Usage:\n" +
			"To enable auto-reply AI: `/auto-reply-ai-config enable`\n" +
			"To disable auto-reply AI: `/auto-reply-ai-config disable`\n" +
			"Example: `/auto-reply-ai-config enable`\n" +
			"\n*Current Auto-Reply AI Config:*\n" +
			fmt.Sprintf("- Enabled: %t\n", auto_reply_stat)

		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}

		return
	}

	if configText == "enable" {
		enable := h.config.App.AIReply.Enable()
		if enable {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Auto-Reply AI is already enabled."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		err := cfg_struct.UpdateAIReplyConfig(true)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to enable Auto-Reply AI. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Auto-Reply AI has been enabled."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if configText == "disable" {
		enable := h.config.App.AIReply.Enable()
		if !enable {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Auto-Reply AI is already disabled."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		err := cfg_struct.UpdateAIReplyConfig(false)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to disable Auto-Reply AI. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Auto-Reply AI has been disabled."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid command. Usage:\n" +
			"To enable auto-reply AI: `/auto-reply-ai-config enable`\n" +
			"To disable auto-reply AI: `/auto-reply-ai-config disable`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) AIGroupInteractionUpdateConfig(args CommandArgs) {
	cmd := RegisteredCommands().AIGroupInteractionConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	if configText == "" {
		group_interaction_stat := h.config.App.AIGroupInterraction.Enable()
		config_help := "AI Group Interaction Config Command Usage:\n" +
			"To enable AI group interaction: `/ai-group-interaction-config enable`\n" +
			"To disable AI group interaction: `/ai-group-interaction-config disable`\n" +
			"Example: `/ai-group-interaction-config enable`\n" +
			"\n*Current AI Group Interaction Config:*\n" +
			fmt.Sprintf("- Enabled: %t\n", group_interaction_stat)
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if configText == "enable" {
		enable := h.config.App.AIGroupInterraction.Enable()
		if enable {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("AI Group Interaction is already enabled."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		err := cfg_struct.UpdateAIGroupInteractionConfig(true)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to enable AI Group Interaction. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("AI Group Interaction has been enabled."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if configText == "disable" {
		enable := h.config.App.AIGroupInterraction.Enable()
		if !enable {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("AI Group Interaction is already disabled."),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		err := cfg_struct.UpdateAIGroupInteractionConfig(false)
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to disable AI Group Interaction. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("AI Group Interaction has been disabled."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid command. Usage:\n" +
			"To enable AI group interaction: `/ai-group-interaction-config enable`\n" +
			"To disable AI group interaction: `/ai-group-interaction-config disable`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}

}

func (h *Agents) AgentModelUpdateConfig(args CommandArgs) {
	cmd := RegisteredCommands().AgentModelConfig
	configText := strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))
	list_providers, err := h.chatCompleter.GetAIDataStatus(context.Background())
	if err != nil {
		list_providers = &ports.AIDataStatus{
			ListAIProviders: []string{"ollama", "openrouter", "nvidia", "anthropic", "google"},
		}
		log.Printf("failed to get AI providers: %v", err)
	}

	if configText == "" {
		config_help := "Agent Model Config Command Usage:\n" +
			"To set main agent model: `/agent-model-config main [provider] [model]`\n" +
			"To set code agent model: `/agent-model-config code [provider] [model]`\n" +
			"To set research agent model: `/agent-model-config research [provider] [model]`\n" +
			"To set reply agent model: `/agent-model-config reply [provider] [model]`\n" +
			"Example: `/agent-model-config main ollama llama3`\n" +
			"\n*Current Agent Model Config:*\n" +
			"- *Main Agent:*\n" +
			fmt.Sprintf("  - Provider: %s\n", h.config.App.AgentConfigModels.AgentModel.AgentProvider()) +
			fmt.Sprintf("  - Model: %s\n", h.config.App.AgentConfigModels.AgentModel.AgentModel()) +
			"- *Code Agent Model:*\n" +
			fmt.Sprintf("  - Provider: %s\n", h.config.App.AgentConfigModels.AgentCodeModel.AgentProvider()) +
			fmt.Sprintf("  - Model: %s\n", h.config.App.AgentConfigModels.AgentCodeModel.AgentModel()) +
			"- *Research Agent Model:*\n" +
			fmt.Sprintf("  - Provider: %s\n", h.config.App.AgentConfigModels.ResearchModel.AgentProvider()) +
			fmt.Sprintf("  - Model: %s\n", h.config.App.AgentConfigModels.ResearchModel.AgentModel()) +
			"- *Reply Agent Model:*\n" +
			fmt.Sprintf("  - Provider: %s\n", h.config.App.AgentConfigModels.ReplyModel.AgentProvider()) +
			fmt.Sprintf("  - Model: %s\n", h.config.App.AgentConfigModels.ReplyModel.AgentModel()) +
			"\n*Supported Providers:*\n- " +
			strings.Join(list_providers.ListAIProviders, "\n- ")
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(config_help),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "main "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 2 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/agent-model-config main [provider] [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		provider := configArgs[0]
		model := configArgs[1]
		err := cfg_struct.UpdateAgentConfigModels(cfg_struct.AgentConfigModelsDB{
			AgentModel: cfg_struct.AgentProvidersAndModelDB{
				AgentProvider: provider,
				AgentModel:    model,
			},
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update main agent model config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Main agent model has been updated to provider '%s' and model '%s'.", provider, model)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "code "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 2 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/agent-model-config code [provider] [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		provider := configArgs[0]
		model := configArgs[1]
		err := cfg_struct.UpdateAgentConfigModels(cfg_struct.AgentConfigModelsDB{
			AgentCodeModel: cfg_struct.AgentProvidersAndModelDB{
				AgentProvider: provider,
				AgentModel:    model,
			},
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update code agent model config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Code agent model has been updated to provider '%s' and model '%s'.", provider, model)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "research "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 2 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/agent-model-config research [provider] [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		provider := configArgs[0]
		model := configArgs[1]
		err := cfg_struct.UpdateAgentConfigModels(cfg_struct.AgentConfigModelsDB{
			ResearchModel: cfg_struct.AgentProvidersAndModelDB{
				AgentProvider: provider,
				AgentModel:    model,
			},
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update research agent model config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Research agent model has been updated to provider '%s' and model '%s'.", provider, model)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if after, ok := strings.CutPrefix(configText, "reply "); ok {
		configArgs := strings.Fields(after)
		if len(configArgs) != 2 {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String("Invalid command format. Usage: `/agent-model-config reply [provider] [model]`"),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		provider := configArgs[0]
		model := configArgs[1]
		err := cfg_struct.UpdateAgentConfigModels(cfg_struct.AgentConfigModelsDB{
			ReplyModel: cfg_struct.AgentProvidersAndModelDB{
				AgentProvider: provider,
				AgentModel:    model,
			},
		})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to update reply agent model config. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Reply agent model has been updated to provider '%s' and model '%s'.", provider, model)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set main agent model: `/agent-model-config main [provider] [model]`\n" +
			"To set code agent model: `/agent-model-config code [provider] [model]`\n" +
			"To set research agent model: `/agent-model-config research [provider] [model]`\n" +
			"To set reply agent model: `/agent-model-config reply [provider] [model]`\n" +
			"Example: `/agent-model-config main ollama llama3`"),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}

func (h *Agents) ShowAllContacts(args CommandArgs) {
	cmd := RegisteredCommands().ShowAllContacts
	_ = strings.TrimSpace(strings.TrimPrefix(args.Text, cmd.Command))

	contacts, err := h.waclient.GetAllContacts(context.Background())
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to get contacts. Error Detail: %s", err.Error())),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if len(contacts) == 0 {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("No contacts found."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	var contactListBuilder strings.Builder
	contactListBuilder.WriteString("Contact List:\n")
	for jid, contact := range contacts {
		if contact.Found {
			fmt.Fprintf(&contactListBuilder, "  - First Name: %s\n", contact.FirstName)
			fmt.Fprintf(&contactListBuilder, "  - Full Name: %s\n", contact.FullName)
			fmt.Fprintf(&contactListBuilder, "  - Push Name: %s\n", contact.PushName)
			fmt.Fprintf(&contactListBuilder, "  - JID/ID/Number: %s\n", jid)
			contactListBuilder.WriteString("\n---\n")
		}
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String(contactListBuilder.String()),
	}); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}
