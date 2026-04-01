package whatsapp

import (
	"context"
	"fmt"

	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"go.mau.fi/whatsmeow/types"
)

func (c *BotClient) GetContact(ctx context.Context, number string) (types.ContactInfo, error) {
	jid := types.NewJID(number, types.DefaultUserServer)
	pkg.StructPrettyPrint(jid, pkg.IoColorCyan)
	ch, err := c.client.Store.Contacts.GetContact(ctx, jid)
	if err != nil {
		return types.ContactInfo{}, fmt.Errorf("failed to get contact: %w", err)
	}
	return ch, nil
}
