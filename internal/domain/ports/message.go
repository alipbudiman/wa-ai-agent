package ports

import "context"

type MessageSender interface {
	SendText(ctx context.Context, to string, text string) error
}
