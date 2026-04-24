package rulebase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func normalizeCommandIntentToken(token string) string {
	token = strings.TrimSpace(strings.ToLower(token))
	if token == "" {
		return ""
	}

	for token != "" {
		r, size := utf8.DecodeRuneInString(token)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		token = token[size:]
	}

	token = strings.TrimFunc(token, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})

	return token
}

func commandSuggestionByName() map[string]string {
	commands := RegisteredCommands()
	out := make(map[string]string, len(commands.All()))
	for _, cmd := range commands.All() {
		commandName := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cmd.Command)), "/")
		if commandName == "" {
			continue
		}
		out[commandName] = cmd.Command
	}

	return out
}

func detectMalformedCommandIntent(text string) (bool, string) {
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" || strings.HasPrefix(trimmedText, "/") {
		return false, ""
	}

	parts := strings.Fields(trimmedText)
	if len(parts) == 0 {
		return false, ""
	}

	firstToken := parts[0]
	normalizedToken := normalizeCommandIntentToken(firstToken)
	if normalizedToken == "" {
		return false, ""
	}

	knownCommands := commandSuggestionByName()
	suggestedCommand, known := knownCommands[normalizedToken]
	if !known {
		return false, ""
	}

	// Exact plain words like "help" are considered command attempts only when
	// they appear as standalone input to avoid hijacking normal sentences.
	if firstToken == normalizedToken && len(parts) > 1 {
		return false, ""
	}

	return true, suggestedCommand
}

func (h *Agents) procTextMessage(v *events.Message, text string) {

	log.Printf("Received message from %s: %s", v.Info.Chat.String(), text)

	if h.isCommand(text) {
		commands := RegisteredCommands()
		if h.CommandHandler(v, text) {
			return
		}

		if err := h.waclient.SendText(
			context.Background(),
			v.Info.Chat.String(),
			&waE2E.Message{
				Conversation: proto.String(fmt.Sprintf("Unknown command. Type %s to see available commands.", commands.Help.Command)),
			},
		); err != nil {
			log.Printf("failed to send unknown command message: %v", err)
		}
		return
	}

	if malformed, suggestedCommand := detectMalformedCommandIntent(text); malformed {
		hintMessage := fmt.Sprintf(
			"It looks like you are trying to use a command. Use '/' prefix. Example: %s",
			suggestedCommand,
		)
		if err := h.waclient.SendText(context.Background(), v.Info.Chat.String(), &waE2E.Message{
			Conversation: proto.String(hintMessage),
		}); err != nil {
			log.Printf("failed to send malformed command hint: %v", err)
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
		key := normalizeVCardPropertyName(parts[0])

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

func normalizeVCardPropertyName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if dotIdx := strings.LastIndex(raw, "."); dotIdx >= 0 && dotIdx+1 < len(raw) {
		raw = raw[dotIdx+1:]
	}

	return strings.ToUpper(strings.TrimSpace(raw))
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

func appendUniqueNonEmpty(values []string, seen map[string]struct{}, raw string) []string {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return values
	}

	if _, exists := seen[normalized]; exists {
		return values
	}

	seen[normalized] = struct{}{}
	return append(values, normalized)
}

func collectContactVCards(v *events.Message) []string {
	if v == nil {
		return nil
	}

	vcards := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	if v.Message != nil && v.Message.ContactMessage != nil {
		vcards = appendUniqueNonEmpty(vcards, seen, v.Message.ContactMessage.GetVcard())
	}

	if v.RawMessage != nil {
		if rawContact := v.RawMessage.GetContactMessage(); rawContact != nil {
			vcards = appendUniqueNonEmpty(vcards, seen, rawContact.GetVcard())
		}
	}

	return vcards
}

func collectContactDisplayNames(v *events.Message) []string {
	if v == nil {
		return nil
	}

	displayNames := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	if v.Message != nil && v.Message.ContactMessage != nil {
		displayNames = appendUniqueNonEmpty(displayNames, seen, v.Message.ContactMessage.GetDisplayName())
	}

	if v.RawMessage != nil {
		if rawContact := v.RawMessage.GetContactMessage(); rawContact != nil {
			displayNames = appendUniqueNonEmpty(displayNames, seen, rawContact.GetDisplayName())
		}
	}

	return displayNames
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			return normalized
		}
	}

	return ""
}

func (h *Agents) procContactMessage(v *events.Message) *ContactVCardInfo {
	if v == nil {
		return nil
	}

	vcards := collectContactVCards(v)
	if len(vcards) == 0 {
		return nil
	}

	var (
		info       *ContactVCardInfo
		parseError error
	)

	for _, vcard := range vcards {
		info, parseError = parseContactVCard(vcard)
		if parseError == nil {
			break
		}
	}

	if parseError != nil || info == nil {
		if err := h.waclient.SendText(
			context.Background(),
			v.Info.Chat.String(),
			&waE2E.Message{
				Conversation: proto.String("Sorry, I couldn't parse the contact information you sent. Please make sure it's a valid vCard format."),
			},
		); err != nil {
			log.Printf("failed to send unknown command message: %v", err)
		}
		if parseError != nil {
			log.Printf("failed to parse contact vcard: %v", parseError)
		}
		return nil
	}

	if info.FN == "" {
		if displayName := firstNonEmpty(collectContactDisplayNames(v)); displayName != "" {
			info.FN = displayName
		}
	}

	if info.N == "" && info.FN != "" {
		info.N = info.FN
	}

	info.N = strings.ReplaceAll(info.N, ";", " ")

	log.Printf("Received contact info:\nName: %s\nFull Name: %s\nPhone: %s", info.N, info.FN, info.WAID)

	return info
}

func contactCardStringFormat(contact_info *ContactVCardInfo) string {
	return fmt.Sprintf("Contact info:\nName: %s\nFull Name: %s\nPhone: %s", contact_info.N, contact_info.FN, contact_info.WAID)
}
