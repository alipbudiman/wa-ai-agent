package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type Sender struct {
	client *whatsmeow.Client
}

func NewSender(client *whatsmeow.Client) (*Sender, error) {
	if client == nil {
		return nil, errors.New("whatsapp client is required")
	}

	return &Sender{client: client}, nil
}

func (s *Sender) SendText(ctx context.Context, to string, text string) error {
	if s == nil || s.client == nil {
		return errors.New("whatsapp client is not initialized")
	}

	to = strings.TrimSpace(to)
	text = strings.TrimSpace(text)
	if to == "" || text == "" {
		return fmt.Errorf("recipient and text are required")
	}

	jid, err := types.ParseJID(to)
	if err != nil {
		return fmt.Errorf("invalid recipient JID: %w", err)
	}
	if jid.IsEmpty() {
		return fmt.Errorf("recipient JID is empty")
	}

	_, err = s.client.SendMessage(ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}
