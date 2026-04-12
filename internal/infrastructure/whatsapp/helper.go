package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
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

func (s *BotClient) detectMediaTypeFromFilePath(filePath string) (whatsmeow.MediaType, string, error) {
	// NOTE: Cek extension dulu untuk deteksi cepat.
	ext := strings.ToLower(filepath.Ext(filePath))
	mimeType := strings.ToLower(mime.TypeByExtension(ext))
	if i := strings.Index(mimeType, ";"); i >= 0 {
		mimeType = mimeType[:i]
	}

	// NOTE: Sniff header file agar tidak hanya bergantung extension.
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	header := make([]byte, 512)
	n, _ := f.Read(header)
	sniffed := strings.ToLower(http.DetectContentType(header[:n]))
	if i := strings.Index(sniffed, ";"); i >= 0 {
		sniffed = sniffed[:i]
	}

	// EXPLANATION: Kalau extension tidak jelas, pakai hasil sniff.
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = sniffed
	}

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return whatsmeow.MediaImage, mimeType, nil
	case strings.HasPrefix(mimeType, "video/"):
		return whatsmeow.MediaVideo, mimeType, nil
	case strings.HasPrefix(mimeType, "audio/"):
		return whatsmeow.MediaAudio, mimeType, nil
	default:
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		return whatsmeow.MediaDocument, mimeType, nil
	}
}
