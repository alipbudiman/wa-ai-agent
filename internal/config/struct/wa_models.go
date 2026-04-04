package cfg_struct

type WhatsAppConfig struct {
	Dialect        string
	SessionDSN     string
	DBLogLevel     string
	ClientLogLevel string
}

type OpenRouterConfig struct {
	APIKey string
	Model  string
	URL    string
}

type AIAgentConfig struct {
	OpenRouter OpenRouterConfig
}

type ModelRouting struct {
	ToolsSelection AIAgentConfig
}

type LangSmithConfig struct {
	APIKey      string
	ProjectName string
}
