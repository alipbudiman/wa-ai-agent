package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	cloud_api_providers "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	eventsrunner "github.com/alipbudiman/wa-ai-agent.git/internal/presentation/run"
	"github.com/alipbudiman/wa-ai-agent.git/internal/usecase/runbot"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present (ignore if missing)
	load_env_file := "use.env.file"
	if _, err := os.Stat(load_env_file); err == nil {
		_ = godotenv.Load(".env")
		log.Printf("Loaded environment variables from %s\n", load_env_file)
	} else {
		log.Printf("No %s file found, skipping load\n", load_env_file)
	}
	cfg := config.Load()
	brainBaseURL := strings.TrimSpace(os.Getenv("BRAIN_API_BASE_URL"))
	if brainBaseURL == "" {
		brainBaseURL = "http://127.0.0.1:8000"
	}

	openRouter, err := cloud_api_providers.NewAIAgent(
		cfg.AIAgent.OpenRouter.APIKey,
		brainBaseURL,
		cfg.AIAgent.OpenRouter.Model,
	)
	if err != nil {
		log.Printf("OpenRouter disabled: %v", err)
	}

	waRunner, err := eventsrunner.ClientRunner(eventsrunner.Config{
		Dialect:        cfg.WhatsApp.Dialect,
		SessionDSN:     cfg.WhatsApp.SessionDSN,
		DBLogLevel:     cfg.WhatsApp.DBLogLevel,
		ClientLogLevel: cfg.WhatsApp.ClientLogLevel,
		ChatCompleter:  openRouter,
		ChatModel:      cfg.AIAgent.OpenRouter.Model,
		LangSmith:      cfg.LangSmith,
	})
	if err != nil {
		log.Fatalf("failed to initialize WhatsApp runner: %v", err)
	}

	app := runbot.NewService(waRunner)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatalf("failed to run bot: %v", err)
	}
}
