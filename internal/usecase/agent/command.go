package rulebase

import (
	"context"
	"fmt"
	"log"
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
	`
)

type CommandCollection struct {
	Help     CommandInfo
	PingPong CommandInfo
}

func registeredCommands() CommandCollection {
	return CommandCollection{
		Help: CommandInfo{
			Name:        "Help",
			Description: "Displays available commands.",
			Command:     "/help",
			Function:    (*RuleBase).Help,
		},
		PingPong: CommandInfo{
			Name:        "Ping Pong",
			Description: "Responds with 'Pong!' to test connectivity.",
			Command:     "/ping",
			Function:    (*RuleBase).Ping,
		},
	}
}

func (c CommandCollection) All() []CommandInfo {
	return []CommandInfo{c.Help, c.PingPong}
}

type CommandAction func(*RuleBase, CommandArgs)

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

func CommandString() string {
	commands := registeredCommands()
	var menuText strings.Builder
	menuText.WriteString("Available Commands:\n")
	for _, cmd := range commands.All() {
		menuText.WriteString(cmd.Command + " - " + cmd.Description + "\n")
	}
	menuText.WriteString("\nUse the command followed by any required parameters to execute.")
	menuText.WriteString("\nExample: /ping")
	fmt.Fprintf(&menuText, "\n\nAI Ability:\n%s", ai_ability)
	return menuText.String()
}

func (h *RuleBase) Help(args CommandArgs) {
	menuText := CommandString()
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), menuText); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}

}

func (h *RuleBase) Ping(args CommandArgs) {
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), "Pong!"); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}
}
