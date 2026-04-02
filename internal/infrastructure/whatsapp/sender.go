package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func (s *BotClient) SendText(ctx context.Context, to string, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.client == nil {
		return errors.New("whatsapp client is not initialized")
	}

	to = strings.TrimSpace(to)
	text = strings.TrimSpace(text)
	if to == "" || text == "" {
		return fmt.Errorf("recipient and text are required")
	}

	jid, err := parseTargetJID(to)
	if err != nil {
		return err
	}

	_, err = s.client.SendMessage(ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}
