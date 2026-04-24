package ports

type AIDataStatus struct {
	ListAIProviders []string `json:"list_ai_providers"`
}

type InjectAIConversationResponse struct {
	Success bool `json:"success"`
}
