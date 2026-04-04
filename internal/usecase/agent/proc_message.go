package rulebase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types/events"
)

type ContactVCardInfo struct {
	N    string
	FN   string
	WAID string
}

func (h *Agents) procTextMessage(v *events.Message) {
	text, err := h.handleMessage(v)
	if err != nil {
		if errors.Is(err, errIgnoreOwnMessage) || errors.Is(err, errEmptyTextMessage) {
			return
		}
		return
	}

	log.Printf("Received message from %s: %s", v.Info.Chat.String(), text)

	if h.isCommand(text) {
		commands := registeredCommands()
		if h.CommandHandler(v, text) {
			return
		}

		if err := h.waclient.SendText(
			context.Background(),
			v.Info.Chat.String(),
			fmt.Sprintf("Unknown command. Type %s to see available commands.", commands.Help.Command),
		); err != nil {
			log.Printf("failed to send unknown command message: %v", err)
		}
		return
	}
	h.ChatHandler(v, text)
}

func parseContactVCard(vcard string) (*ContactVCardInfo, error) {
	lines := unfoldVCardLines(vcard)
	if len(lines) == 0 {
		return nil, errors.New("empty vcard")
	}

	out := &ContactVCardInfo{}

	for _, line := range lines {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}

		left := line[:idx]
		value := line[idx+1:]

		parts := strings.Split(left, ";")
		key := strings.ToUpper(strings.TrimSpace(parts[0]))

		switch key {
		case "N":
			out.N = unescapeVCardValue(value)
		case "FN":
			out.FN = unescapeVCardValue(value)
		case "TEL":
			if out.WAID == "" {
				out.WAID = parseWAID(parts[1:], value)
			}
		}
	}

	if out.N == "" && out.FN == "" && out.WAID == "" {
		return nil, errors.New("vcard does not contain N, FN, or waid")
	}

	return out, nil
}

func unfoldVCardLines(vcard string) []string {
	normalized := strings.ReplaceAll(vcard, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rawLines := strings.Split(normalized, "\n")

	lines := make([]string, 0, len(rawLines))
	for _, rawLine := range rawLines {
		line := strings.TrimRight(rawLine, " \t")
		if line == "" {
			continue
		}

		if len(lines) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}

		lines = append(lines, strings.TrimSpace(line))
	}

	return lines
}

func parseWAID(params []string, telValue string) string {
	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		eqIdx := strings.Index(param, "=")
		if eqIdx < 0 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(param[:eqIdx]))
		val := strings.TrimSpace(param[eqIdx+1:])
		if key == "waid" {
			if digits := digitsOnly(val); digits != "" {
				return digits
			}
			return val
		}
	}

	return digitsOnly(telValue)
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unescapeVCardValue(value string) string {
	replacer := strings.NewReplacer(
		"\\n", "\n",
		"\\N", "\n",
		"\\,", ",",
		"\\;", ";",
		"\\\\", "\\",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func (h *Agents) procContactMessage(v *events.Message) *ContactVCardInfo {
	if v == nil || v.Message == nil || v.Message.ContactMessage == nil {
		return nil
	}

	vcard := strings.TrimSpace(v.Message.ContactMessage.GetVcard())
	if vcard == "" {
		return nil
	}

	info, err := parseContactVCard(vcard)
	if err != nil {
		if err := h.waclient.SendText(
			context.Background(),
			v.Info.Chat.String(),
			"Received a contact message, but failed to parse contact info.",
		); err != nil {
			log.Printf("failed to send unknown command message: %v", err)
		}
		return nil
	}

	log.Printf("Received contact info:\nName: %s\nFull Name: %s\nPhone: %s", info.N, info.FN, info.WAID)

	return info
}
