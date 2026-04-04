package whatsapp

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func (c *BotClient) ParseJID(target string) (types.JID, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return types.JID{}, fmt.Errorf("recipient is required")
	}

	if strings.Contains(target, "@") {
		jid, err := types.ParseJID(target)
		if err != nil {
			return types.JID{}, fmt.Errorf("invalid recipient JID: %w", err)
		}
		if jid.IsEmpty() {
			return types.JID{}, fmt.Errorf("recipient JID is empty")
		}
		return jid, nil
	}

	return types.NewJID(target, types.DefaultUserServer), nil
}
