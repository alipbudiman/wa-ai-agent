package rulebase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	ai_providers_api "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/databases"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"github.com/alipbudiman/wa-ai-agent.git/skilldev"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
)

func init() {
	capabilityDescription = readCapabilityDescription()
	menuString = CommandString()

	md := skilldev.NewMarkdownWriter()
	md.H1("WhatsApp AI Agent Command Menu").Text("Here are the available commands you can use to interact with the WhatsApp AI Agent:").BulletList(func() []string {
		commands := registeredCommands()
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

func registeredCommands() CommandCollection {
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
	}
}

func (c CommandCollection) All() []CommandInfo {
	return []CommandInfo{
		c.Help,
		c.PingPong,
		c.About,
		c.SendPendingMessage,
		c.UnsendPendingMessage,
		c.SendPendingFile,
		c.UnsendPendingFile,
	}
}

func CommandString() string {
	commands := registeredCommands()
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

func (h *Agents) SendPendingMessageFromAgent(args CommandArgs) {
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
		fmt.Sprintf("Send Message to %s has been sent.",
			name),
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
		fmt.Sprintf("Send message to %s has been canceled.", name),
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

func (h *Agents) SendPendingFileFromAgent(args CommandArgs) {
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

	new_jid, err := h.waclient.ParseJID(pending_file.PendingNumber)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Invalid pending number format: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
	}

	err = h.waclient.SendFile(context.Background(), new_jid.String(), pending_file.PendingFile, pending_file.PendingCaption)
	if err != nil {
		if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(fmt.Sprintf("Failed to send pending file: %v", err)),
		}); err != nil {
			log.Printf("failed to send reply message: %v", err)
		}
		return
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
		fmt.Sprintf("Send File to %s has been sent.",
			name),
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
		fmt.Sprintf("Send file to %s has been canceled.", name),
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