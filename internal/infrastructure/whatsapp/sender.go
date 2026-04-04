package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
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

func (s *BotClient) SendText(ctx context.Context, to string, text string) error {

	jid, err := s.guardSenderInitialized(ctx, to, text)
	if err != nil {
		return err
	}

	_, err = s.client.SendMessage(ctx, *jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

func (s *BotClient) SendQuoteMessage(ctx context.Context, to, text, quote_msg string) error {
	jid, err := s.guardSenderInitialized(ctx, to, text)
	if err != nil {
		return err
	}

	_, err = s.client.SendMessage(ctx, *jid, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				QuotedMessage: &waE2E.Message{Conversation: proto.String(quote_msg)},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("send quote message: %w", err)
	}

	return nil
}
