package rulebase

import "log"

func handleMessageEventGuard(h *RuleBase) bool {
	if h == nil {
		log.Printf("rule base is not initialized")
		return false
	}

	if h.responder == nil {
		log.Printf("responder is not initialized")
		return false
	}

	if h.waclient == nil {
		log.Printf("whatsapp client is not initialized")
		return false
	}

	return true
}
