// @title WA AI Agent HTTP API
// @version 1.0
// @description Gin HTTP API for WhatsApp bot utilities and integrations.
// @host localhost:8080
// @BasePath /
// @schemes http
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alipbudiman/wa-ai-agent.git/internal/config"
	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	cloud_api_providers "github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/ai/api"
	route "github.com/alipbudiman/wa-ai-agent.git/internal/presentation/http"
	eventsrunner "github.com/alipbudiman/wa-ai-agent.git/internal/presentation/run"
	"github.com/alipbudiman/wa-ai-agent.git/internal/usecase/runbot"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"
	"go.mau.fi/whatsmeow"
)

const swaggerJSONPath = "internal/presentation/http/docs/swagger.json"

type runtimeSwaggerDoc struct {
	doc string
}

func (r runtimeSwaggerDoc) ReadDoc() string {
	return r.doc
}

var (
	client   *whatsmeow.Client
	waClient ports.WhatsAppClient
)

func botRunner(cfg *config.Config) (*eventsrunner.Runner, error) {
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
		return nil, err
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
		return nil, err
	}

	client = waRunner.Client()
	waClient = waRunner.WhatsAppClient()

	return waRunner, nil
}

func compileSwaggerDocs(ctx context.Context) error {
	cmd := exec.CommandContext(
		ctx,
		"go",
		"run",
		"github.com/swaggo/swag/cmd/swag@v1.16.4",
		"init",
		"-g",
		"main.go",
		"-o",
		"internal/presentation/http/docs",
		"--parseInternal",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compile swagger docs: %w", err)
	}

	docBytes, err := os.ReadFile(swaggerJSONPath)
	if err != nil {
		return fmt.Errorf("read generated swagger json: %w", err)
	}

	if swag.GetSwagger(swag.Name) == nil {
		swag.Register(swag.Name, runtimeSwaggerDoc{doc: string(docBytes)})
	}

	return nil
}

func serverRunner(ctx context.Context, cfg *config.Config) error {
	gin.SetMode(string(cfg.GinServer.AppMod))
	router := gin.New()
	router.Use(gin.Recovery())

	srv := &http.Server{
		Addr:              ":" + cfg.GinServer.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if client == nil || waClient == nil {
		return fmt.Errorf("whatsapp client is not initialized")
	}

	if err := compileSwaggerDocs(ctx); err != nil {
		return err
	}

	router = route.RouterV1(waClient, cfg, router)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Start server in background; shutdown is tied to main context.
	go func() {
		log.Printf("[http] server starting on http://localhost:%s\n", cfg.GinServer.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[http] listen error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		log.Println("server shutting down...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server forced to shutdown: %v", err)
			return
		}

		log.Println("server exited cleanly")
	}()

	return nil
}

func brainRunner() {}

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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	waRunner, err := botRunner(&cfg)
	if err != nil {
		log.Fatalf("failed to initialize bot runner: %v", err)
		return
	}

	if err := serverRunner(ctx, &cfg); err != nil {
		log.Fatalf("failed to initialize http server runner: %v", err)
		return
	}

	app := runbot.NewService(waRunner)

	if err := app.Run(ctx); err != nil {
		log.Fatalf("failed to run bot: %v", err)
	}
}
