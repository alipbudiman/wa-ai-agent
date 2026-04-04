package handler

import (
	"context"
	"log"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"go.mau.fi/whatsmeow/types"
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

func SendMessage(waClient ports.WhatsAppClient, number string, message string) error {
	err := waClient.SendText(context.Background(), number, message)
	if err != nil {
		log.Printf("failed to send message to %s: %v", number, err)
		return err
	}
	log.Printf("message sent to %s successfully", number)
	return nil
}

func BroadcastMessage(waClient ports.WhatsAppClient, message string) error {
	err := waClient.SendText(context.Background(), types.StatusBroadcastJID.String(), message)
	if err != nil {
		log.Printf("failed to broadcast message: %v", err)
		return err
	}
	log.Printf("message broadcast successfully")
	return nil
}
