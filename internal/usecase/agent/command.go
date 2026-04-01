package rulebase

import (
	"strings"
	"context"
	"log"

	"go.mau.fi/whatsmeow/types/events"
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

func (h *RuleBase) Help(args CommandArgs) {
	commands := registeredCommands()
	var menuText strings.Builder; menuText.WriteString("Available Commands:\n")
	for _, cmd := range commands.All() {
		menuText .WriteString(cmd.Command + " - " + cmd.Description + "\n")
	}
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), menuText.String()); err != nil {
		log.Printf("failed to send reply message: %v", err)
	}

}

func (h *RuleBase) Ping(args CommandArgs) {
	if err := h.waclient.SendText(context.Background(), args.Evt.Info.Chat.String(), "Pong!"); err != nil {
		log.Printf("failed to send reply message: %v", err)
		return
	}
}
