package whatsapp

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

func (c *BotClient) GetContact(ctx context.Context, number string) (types.ContactInfo, error) {
	jid, err := parseTargetJID(number)
	if err != nil {
		return types.ContactInfo{}, err
	}

	ch, err := c.client.Store.Contacts.GetContact(ctx, jid.ToNonAD())
	if err != nil {
		return types.ContactInfo{}, fmt.Errorf("failed to get contact: %w", err)
	}
	return ch, nil
}
