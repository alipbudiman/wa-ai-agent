package whatsapp

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

func (c *BotClient) GetContact(ctx context.Context, number string) (types.ContactInfo, error) {
	jid, err := c.ParseJID(number)
	if err != nil {
		return types.ContactInfo{}, err
	}

	ch, err := c.client.Store.Contacts.GetContact(ctx, jid.ToNonAD())
	if err != nil {
		return types.ContactInfo{}, fmt.Errorf("failed to get contact: %w", err)
	}
	return ch, nil
}

func (c *BotClient) GetSelfJID(ctx context.Context) (types.JID, error) {
	return c.client.Store.GetJID(), nil
}
