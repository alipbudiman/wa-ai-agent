package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func (s *BotClient) SendText(ctx context.Context, to string, message *waE2E.Message) error {
	jid, err := s.guardSenderInitialized(ctx, to, *message.Conversation)
	if err != nil {
		return err
	}

	chunks := s.splitLongMessage(*message.Conversation, MAX_MESSAGE_LENGTH)
	for _, chunk := range chunks {
		// Send each chunk as a separate message
		message.Conversation = proto.String(chunk)

		_, err := s.client.SendMessage(ctx, *jid, message)
		if err != nil {
			return fmt.Errorf("send message chunk: %w", err)
		}
	}

	return nil
}

func (s *BotClient) SendContact(ctx context.Context, to string, target ports.ContactCard, context_info *waE2E.ContextInfo) error {

	jid, err := s.guardSenderInitialized(ctx, to, target.Number)
	if err != nil {
		return err
	}

	var content strings.Builder
	content.WriteString("BEGIN:VCARD\n")
	content.WriteString("VERSION:3.0\n")
	content.WriteString(fmt.Sprintf("N:%s\n", s.nformatContact(target.FullName)))
	content.WriteString(fmt.Sprintf("FN:%s\n", target.FullName))
	content.WriteString(fmt.Sprintf("item1.TEL;waid=%s:%s\n", target.Number, s.formatPhoneNumber(target.Number)))
	content.WriteString("item1.X-ABLabel:Mobile\n")
	content.WriteString("END:VCARD\n")

	name := target.FullName
	if name == "" {
		name = target.Number
	}

	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(name),
			Vcard:       proto.String(content.String()),
			ContextInfo: context_info,
		},
	}

	_, err = s.client.SendMessage(ctx, *jid, msg)
	if err != nil {
		return fmt.Errorf("send contact message: %w", err)
	}

	return nil
}

func (s *BotClient) SendQuoteMessage(ctx context.Context, to, text, quote_msg string) error {
	jid, err := s.guardSenderInitialized(ctx, to, text)
	if err != nil {
		return err
	}

	chunks := s.splitLongMessage(text, MAX_MESSAGE_LENGTH)
	for _, chunk := range chunks {
		// Send each chunk as a separate message

		_, err = s.client.SendMessage(ctx, *jid, &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(chunk),
				ContextInfo: &waE2E.ContextInfo{
					QuotedMessage: &waE2E.Message{Conversation: proto.String(quote_msg)},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("send quote message: %w", err)
		}
	}

	return nil
}

func (s *BotClient) SendFile(ctx context.Context, to string, filePath string, caption string) error {
	jid, err := s.guardSenderInitialized(ctx, to, caption)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	mediaType, mimeType, err := s.detectMediaTypeFromFilePath(filePath)
	if err != nil {
		return fmt.Errorf("detect media type: %w", err)
	}

	uploaded, err := s.client.Upload(ctx, data, mediaType)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}

	msg := &waE2E.Message{}
	fileName := filepath.Base(filePath)

	switch mediaType {
	case whatsmeow.MediaImage:
		msg.ImageMessage = &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Caption:       proto.String(caption),
		}
	case whatsmeow.MediaVideo:
		msg.VideoMessage = &waE2E.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Caption:       proto.String(caption),
		}
	case whatsmeow.MediaAudio:
		msg.AudioMessage = &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
		}
	default:
		// Default as Document (for ZIP, PDF, etc)
		msg.DocumentMessage = &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Title:         proto.String(fileName),
			FileName:      proto.String(fileName),
			Caption:       proto.String(caption),
		}
	}

	_, err = s.client.SendMessage(ctx, *jid, msg)
	if err != nil {
		return fmt.Errorf("send file message: %w", err)
	}

	return nil
}
