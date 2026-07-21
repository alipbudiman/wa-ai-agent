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

	lookupJID := jid.ToNonAD()

	ch, err := c.client.Store.Contacts.GetContact(ctx, lookupJID)
	if err != nil {
		return types.ContactInfo{}, fmt.Errorf("failed to get contact: %w", err)
	}

	if ch.Found {
		return ch, nil
	}

	altJID, err := c.client.Store.GetAltJID(ctx, lookupJID)
	if err != nil {
		return ch, fmt.Errorf("failed to resolve alternate jid for %s: %w", lookupJID, err)
	}
	if altJID.IsEmpty() {
		return ch, nil
	}

	altContact, err := c.client.Store.Contacts.GetContact(ctx, altJID.ToNonAD())
	if err != nil {
		return types.ContactInfo{}, fmt.Errorf("failed to get contact from alternate jid: %w", err)
	}
	if altContact.Found {
		return altContact, nil
	}

	return ch, nil
}

func (c *BotClient) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	contacts, err := c.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all contacts: %w", err)
	}
	return contacts, nil
}

func (c *BotClient) GetSelfJID(ctx context.Context) (types.JID, error) {
	jid := c.client.Store.GetJID()
	if jid.IsEmpty() {
		return types.JID{}, fmt.Errorf("self JID is empty")
	}

	lookupJID := jid.ToNonAD()

	ch, err := c.client.Store.Contacts.GetContact(ctx, lookupJID)
	if err != nil {
		return types.JID{}, fmt.Errorf("failed to get self contact: %w", err)
	}
	if ch.Found {
		return jid, nil
	}

	altJID, err := c.client.Store.GetAltJID(ctx, lookupJID)
	if err != nil {
		return types.JID{}, fmt.Errorf("failed to resolve alternate jid for self: %w", err)
	}
	if altJID.IsEmpty() {
		return jid, nil
	}
	return altJID, nil
}

// GetAltJID resolves the alternate (LID <-> phone-number) JID for the given JID
// using the device's mapping store.
func (c *BotClient) GetAltJID(ctx context.Context, jid types.JID) (types.JID, error) {
	return c.client.Store.GetAltJID(ctx, jid.ToNonAD())
}
