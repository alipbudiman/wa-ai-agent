package handler

import (
	"context"
	"log"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	rulebase "github.com/alipbudiman/wa-ai-agent.git/internal/usecase/agent"
	"go.mau.fi/whatsmeow/types"
)

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

func OpenHelpMenu(waClient ports.WhatsAppClient, number string) error {
	command_str := rulebase.CommandString()
	err := waClient.SendText(context.Background(), number, command_str)
	if err != nil {
		log.Printf("failed to send help menu: %v", err)
		return err
	}
	log.Printf("help menu sent successfully")
	return nil
}
