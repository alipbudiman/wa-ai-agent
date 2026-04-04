package rulebase

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
)

type CommandCollection struct {
	Help     CommandInfo
	PingPong CommandInfo
	About    CommandInfo
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
	}
}

func (c CommandCollection) All() []CommandInfo {
	return []CommandInfo{c.Help, c.PingPong, c.About}
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

func readCapabilityDescription() string {
	paths := []string{"CAPABILITY.md"}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
		paths = append(paths, filepath.Join(repoRoot, "skills", "CAPABILITY.md"))
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		trimmed := strings.TrimSpace(string(content))
		if trimmed != "" {
			return trimmed
		}
	}

	return strings.TrimSpace(ai_ability)
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
	fmt.Fprintf(&menuText, "\n\n%s", readCapabilityDescription())
	return menuText.String()
}

func (h *Agents) Help(args CommandArgs) {
	menuText := CommandString()
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), menuText); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}

}

func (h *Agents) Ping(args CommandArgs) {
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), "Pong!"); err != nil {
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

	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), aboutText.String()); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}
}
