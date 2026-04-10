package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

const (
	MAX_MESSAGE_LENGTH = 50000 // Set the maximum message length for splitting long messages
)

func (s *BotClient) guardSenderInitialized(ctx context.Context, to string, text string) (*types.JID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.client == nil {
		return nil, errors.New("whatsapp client is not initialized")
	}

	to = strings.TrimSpace(to)
	text = strings.TrimSpace(text)
	if to == "" || text == "" {
		return nil, fmt.Errorf("recipient and text are required")
	}

	jid, err := s.ParseJID(to)
	if err != nil {
		return nil, err
	}
	return &jid, nil
}

func (s *BotClient) splitLongMessage(message string, maxLen int) []string {
	if len(message) <= maxLen {
		return []string{message}
	}

	var chunks []string
	remaining := message

	for len(remaining) > 0 {
		// Check if the remaining text fits in one chunk
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}

		// Try to find a good splitting point (newline or space) near the max length
		splitPoint := maxLen

		// Look for a newline character to make a clean break
		lastNewline := strings.LastIndex(remaining[:maxLen], "\n")
		if lastNewline > maxLen/2 {
			// If we found a newline in the latter half of the chunk, split there
			splitPoint = lastNewline + 1 // +1 to include the newline
		} else {
			// Otherwise look for a space to avoid breaking words
			lastSpace := strings.LastIndex(remaining[:maxLen], " ")
			if lastSpace > maxLen/2 {
				splitPoint = lastSpace + 1 // +1 to include the space
			}
		}

		// Add the chunk and remove it from the remaining text
		chunks = append(chunks, remaining[:splitPoint])
		remaining = remaining[splitPoint:]
	}

	return chunks
}
