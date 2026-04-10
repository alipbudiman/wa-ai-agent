package handler

import (
	"context"
	"log"
	"time"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/pkg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func GetSelfInformation(waClient ports.WhatsAppClient) (types.JID, error) {
	self_jid, err := waClient.GetSelfJID(context.Background())
	if err != nil {
		log.Printf("failed to get self jid: %v", err)
		return types.JID{}, err
	}
	log.Printf("self jid: %s", self_jid)
	return self_jid, nil
}

func GetContactInfo(waClient ports.WhatsAppClient, number string) (types.ContactInfo, error) {
	contact, err := waClient.GetContact(context.Background(), number)
	if err != nil {
		log.Printf("failed to get contact info for %s: %v", number, err)
		return types.ContactInfo{}, err
	}
	log.Printf("contact info for %s: %+v", number, contact)
	return contact, nil

}

func SendMessage(waClient ports.WhatsAppClient, manager ports.Manager, number string, message string, store bool) error {
	err := waClient.SendText(context.Background(), number, &waE2E.Message{
		Conversation: proto.String(message),
	})
	if err != nil {
		log.Printf("failed to send message to %s: %v", number, err)
		return err
	}

	contact_info, err := waClient.GetContact(context.Background(), number)
	if err != nil {
		log.Printf("failed to get contact info for %s: %v", number, err)
		return err
	}

	new_jid, err := waClient.ParseJID(number)
	if err != nil {
		log.Printf("failed to parse jid for %s: %v", number, err)
		return err
	}


	if store {
		err = manager.StoreInboxMessage(ports.InboxMessage{
			JID:        new_jid,
			MsgID:      pkg.GenerateUID(),
			FullName:   contact_info.FullName,
			PushName:   contact_info.PushName,
			Number:     new_jid.User,
			Message:    message,
			FromMe:     true,
			ReceivedAt: time.Now().Unix(),
			Read:       true,
		})
		if err != nil {
			if err := waClient.SendText(context.Background(), number, &waE2E.Message{
				Conversation: proto.String(err.Error()),
			}); err != nil {
				log.Printf("failed to send reply message: %v", err)
			}
		}
	}
	log.Printf("message sent to %s successfully", number)
	return nil
}

func BroadcastMessage(waClient ports.WhatsAppClient, message string) error {
	err := waClient.SendText(context.Background(), types.StatusBroadcastJID.String(), &waE2E.Message{
		Conversation: proto.String(message),
	})
	if err != nil {
		log.Printf("failed to broadcast message: %v", err)
		return err
	}
	log.Printf("message broadcast successfully")
	return nil
}
