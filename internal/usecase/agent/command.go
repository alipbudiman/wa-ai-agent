package rulebase

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
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
		c.AutoReplyAIConfig,
		c.AIGroupInteractionConfig,
		c.OllamaConfig,
		c.OpenRouterConfig,
		c.NvidiaConfig,
		c.AnthropicConfig,
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
	fmt.Fprintf(&menuText, "\n\n%s", capabilityDescription)
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

	if capability := strings.TrimSpace(readCapabilityDescription()); capability != "" {
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

	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: fmt.Sprintf("Send Message to %s has been sent.", name),
			},
		},
		true,
		ports.HISTORY,
		args.Evt.Info.Sender,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send confirmation message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ Message has been sent by the agent"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	} else {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Failed to notify agent about sent pending message."),
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
		args.Evt.Info.Sender,
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

	replyStatus, err := h.responder.Reply(
		context.Background(),
		[]openai.ChatCompletionMessage{
			{
				Role:    h.isFromGroup(args.Evt),
				Content: fmt.Sprintf("Send File to %s has been sent.", name),
			},
		},
		true,
		ports.HISTORY,
		args.Evt.Info.Sender,
	)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send confirmation message: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
		return
	}

	if replyStatus == ai_providers_api.SUCCESS {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("⚙️ File has been sent by the agent"),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	} else {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Failed to notify agent about sent pending file."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
			return
		}
	}
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
		args.Evt.Info.Sender,
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
			ListAIProviders: []string{"ollama", "openrouter", "nvidia", "anthropic"},
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
		args.Evt.Info.Sender,
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
		args.Evt.Info.Sender,
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
			"To set API key: `/openrouter-config api_key [your_api_key]`\n" +
			"To set model: `/openrouter-config model [model_name]`\n" +
			"Example: `/openrouter-config api_key sk-xxxxxx`\n" +
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

	if after, ok := strings.CutPrefix(configText, "api_key "); ok {
		apiKey := strings.TrimSpace(after)
		err := cfg_struct.UpdateOpenRouterConfig(cfg_struct.OpenRouterConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set OpenRouter API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("OpenRouter API key has been updated."),
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
			"To set OpenRouter API key: `/openrouter-config api_key [your_api_key]`\n" +
			"Example: `/openrouter-config api_key sk-xxxxxx`"),
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
			"To set API key: `/nvidia-config api_key [your_api_key]`\n" +
			"To set model: `/nvidia-config model [model_name]`\n" +
			"Example: `/nvidia-config api_key sk-xxxxxx`\n" +
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

	if after, ok := strings.CutPrefix(configText, "api_key "); ok {
		apiKey := strings.TrimSpace(after)
		err := cfg_struct.UpdateNvidiaConfig(cfg_struct.NvidiaConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set NVIDIA API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("NVIDIA API key has been updated."),
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
			"To set NVIDIA API key: `/nvidia-config api_key [your_api_key]`\n" +
			"Example: `/nvidia-config api_key sk-xxxxxx`"),
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
			"To set API key: `/anthropic-config api_key [your_api_key]`\n" +
			"To set model: `/anthropic-config model [model_name]`\n" +
			"To set Base URL: `/anthropic-config base_url [url]`\n" +
			"To set AuthToken: `/anthropic-config auth_token [token]`\n" +
			"Example: `/anthropic-config api_key sk-xxxxxx`\n" +
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

	if after, ok := strings.CutPrefix(configText, "api_key "); ok {
		apiKey := strings.TrimSpace(after)
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{APIKey: apiKey})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic API key. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Anthropic API key has been updated."),
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

	if after, ok := strings.CutPrefix(configText, "base_url "); ok {
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

	if after, ok := strings.CutPrefix(configText, "auth_token "); ok {
		authToken := strings.TrimSpace(after)
		err := cfg_struct.UpdateAnthropicConfig(cfg_struct.AnthropicConfig{AuthToken: authToken})
		if err != nil {
			if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Failed to set Anthropic Auth Token. Error Detail: %s", err.Error())),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
			return
		}
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String("Anthropic Auth Token has been updated."),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
		Conversation: proto.String("Invalid config command. Usage:\n" +
			"To set Anthropic API key: `/anthropic-config api_key [your_api_key]`\n" +
			"Example: `/anthropic-config api_key sk-xxxxxx`"),
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
