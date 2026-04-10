package whatsapp

import (
	"context"
	"fmt"
	"strings"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
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
