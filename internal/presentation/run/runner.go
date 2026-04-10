package eventsrunner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"

	appconfig "github.com/alipbudiman/wa-ai-agent.git/internal/config"
	"github.com/mdp/qrterminal/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	ports "github.com/alipbudiman/wa-ai-agent.git/internal/domain"
	"github.com/alipbudiman/wa-ai-agent.git/internal/infrastructure/whatsapp"
	eventshandler "github.com/alipbudiman/wa-ai-agent.git/internal/presentation/handler"
	rulebase "github.com/alipbudiman/wa-ai-agent.git/internal/usecase/agent"
	"github.com/alipbudiman/wa-ai-agent.git/internal/usecase/chat"
	"github.com/alipbudiman/wa-ai-agent.git/internal/usecase/manager"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Config struct {
	ChatCompleter ports.ChatCompleter
	AppConfig     *appconfig.Config
}

type Runner struct {
	client        *whatsmeow.Client
	container     *sqlstore.Container
	responder     ports.AgentResponder
	waclient      ports.WhatsAppClient
	manager       ports.Manager
	chatCompleter ports.ChatCompleter
}

func ClientRunner(cfg Config) (*Runner, error) {
	if cfg.AppConfig.WhatsApp.Dialect == "" {
		cfg.AppConfig.WhatsApp.Dialect = "sqlite"
	}
	if cfg.AppConfig.WhatsApp.SessionDSN == "" {
		cfg.AppConfig.WhatsApp.SessionDSN = "file:wa_session.db?_pragma=foreign_keys(1)"
	}
	if cfg.AppConfig.WhatsApp.DBLogLevel == "" {
		cfg.AppConfig.WhatsApp.DBLogLevel = "INFO"
	}
	if cfg.AppConfig.WhatsApp.ClientLogLevel == "" {
		cfg.AppConfig.WhatsApp.ClientLogLevel = "INFO"
	}

	dbLogger := waLog.Stdout("Database", cfg.AppConfig.WhatsApp.DBLogLevel, true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, cfg.AppConfig.WhatsApp.Dialect, cfg.AppConfig.WhatsApp.SessionDSN, dbLogger)
	if err != nil {
		return nil, fmt.Errorf("init sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("get device store: %w", err)
	}

	log.Printf("device store: %+v", deviceStore.GetJID().String())

	clientLogger := waLog.Stdout("Client", cfg.AppConfig.WhatsApp.ClientLogLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLogger)

	runner := &Runner{
		client:    client,
		container: container,
	}

	waclient, err := whatsapp.NewBotClient(client)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("init message sender: %w", err)
	}

	runner.waclient = waclient
	runner.chatCompleter = cfg.ChatCompleter
	if cfg.ChatCompleter != nil {
		runner.responder = chat.NewService(waclient, cfg.ChatCompleter)
	}

	manager, err := manager.NewManager(client, waclient, *cfg.AppConfig)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("init manager: %w", err)
	}
	runner.manager = manager

	var messageHandler ports.MessageHandler
	if runner.responder != nil {
		messageHandler, err = rulebase.NewAgents(runner.waclient, runner.responder, runner.manager, *cfg.AppConfig)
		if err != nil {
			_ = container.Close()
			return nil, fmt.Errorf("init message handler: %w", err)
		}
	}

	runner.client.AddEventHandler(eventshandler.EventTrace(cfg.AppConfig, runner.waclient, messageHandler, manager))

	return runner, nil
}

func (r *Runner) Run(ctx context.Context) error {
	store.DeviceProps.Os = proto.String(cases.Title(language.English).String(fmt.Sprintf("Chrome (%v)", runtime.GOOS)))
	store.DeviceProps.RequireFullSync = proto.Bool(true)
	if r == nil || r.client == nil {
		return errors.New("whatsapp client is not initialized")
	}

	if r.client.Store.ID == nil {
		qrChan, err := r.client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("get QR channel: %w", err)
		}

		go r.consumeQR(qrChan)
	}

	if err := r.client.Connect(); err != nil {
		if r.container != nil {
			_ = r.container.Close()
		}
		return fmt.Errorf("connect whatsapp client: %w", err)
	}

	<-ctx.Done()
	r.client.Disconnect()

	if r.container != nil {
		if err := r.container.Close(); err != nil {
			log.Printf("failed to close sqlstore: %v", err)
		}
	}

	return nil
}

func (r *Runner) consumeQR(qrChan <-chan whatsmeow.QRChannelItem) {
	for evt := range qrChan {
		switch evt.Event {
		case whatsmeow.QRChannelEventCode:
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, log.Writer())
			fmt.Println("Scan the QR code above to log in to WhatsApp")
		case whatsmeow.QRChannelEventError:
			log.Printf("QR error: %v", evt.Error)
		default:
			log.Printf("QR event: %s", evt.Event)
		}
	}
}

func (r *Runner) Client() *whatsmeow.Client {
	if r == nil {
		return nil
	}

	return r.client
}

func (r *Runner) WhatsAppClient() ports.WhatsAppClient {
	if r == nil {
		return nil
	}

	return r.waclient
}

func (r *Runner) Manager() ports.Manager {
	if r == nil {
		return nil
	}

	return r.manager
}

func (r *Runner) ChatCompleter() ports.ChatCompleter {
	if r == nil {
		return nil
	}

	return r.chatCompleter
}
