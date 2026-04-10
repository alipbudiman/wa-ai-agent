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

func (c *BotClient) nformatContact(name string) string {
	Nname := strings.Split(name, " ")
	if len(Nname) == 1 {
		return ";" + Nname[0] + ";;;"
	} else if len(Nname) == 2 {
		return Nname[1] + ";" + Nname[0] + ";;;"
	} else {
		Nbody := strings.ReplaceAll(name, " "+Nname[+len(Nname)-1], "")
		Nbody = strings.ReplaceAll(Nbody, " "+Nname[len(Nname)-2], "")
		return Nname[len(Nname)-1] + ";" + Nbody + ";" + Nname[len(Nname)-2] + ";;"
	}
}

func (c *BotClient) formatPhoneNumber(number string) string {
	number = strings.ReplaceAll(number, "+", "")
	number = strings.ReplaceAll(number, "-", "")
	number = strings.ReplaceAll(number, " ", "")
	if strings.HasPrefix(number, "62") {
		if len(number) == 13 {
			number = strings.TrimPrefix(number, "62")
			number = "+62 " + number[:3] + "-" + number[3:7] + "-" + number[7:]
			return number
		} else if len(number) == 11 {
			number = strings.TrimPrefix(number, "62")
			number = "+62 " + number[:3] + "-" + number[3:6] + "-" + number[6:]
			return number
		} else {
			return ""
		}

	}
	return ""
}
