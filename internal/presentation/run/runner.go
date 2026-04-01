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
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Config struct {
	Dialect        string
	SessionDSN     string
	DBLogLevel     string
	ClientLogLevel string
	ChatCompleter  ports.ChatCompleter
	ChatModel      string
	LangSmith      appconfig.LangSmithConfig
}

type Runner struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	responder ports.AgentResponder
	waclient  ports.WhatsAppClient
}

func ClientRunner(cfg Config) (*Runner, error) {
	if cfg.Dialect == "" {
		cfg.Dialect = "sqlite"
	}
	if cfg.SessionDSN == "" {
		cfg.SessionDSN = "file:wa_session.db?_pragma=foreign_keys(1)"
	}
	if cfg.DBLogLevel == "" {
		cfg.DBLogLevel = "INFO"
	}
	if cfg.ClientLogLevel == "" {
		cfg.ClientLogLevel = "INFO"
	}

	dbLogger := waLog.Stdout("Database", cfg.DBLogLevel, true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, cfg.Dialect, cfg.SessionDSN, dbLogger)
	if err != nil {
		return nil, fmt.Errorf("init sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("get device store: %w", err)
	}

	clientLogger := waLog.Stdout("Client", cfg.ClientLogLevel, true)
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
	if cfg.ChatCompleter != nil {
		runner.responder = chat.NewService(waclient, cfg.ChatCompleter, cfg.ChatModel)
	}

	var messageHandler ports.MessageHandler
	if runner.responder != nil {
		messageHandler, err = rulebase.NewRuleBase(runner.waclient, runner.responder, appconfig.Config{
			LangSmith: cfg.LangSmith,
		})
		if err != nil {
			_ = container.Close()
			return nil, fmt.Errorf("init message handler: %w", err)
		}
	}

	runner.client.AddEventHandler(eventshandler.EventTrace(messageHandler))

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
