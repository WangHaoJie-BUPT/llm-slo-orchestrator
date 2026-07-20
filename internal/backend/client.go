package backend

import (
	"context"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
)

type Event struct {
	Data []byte
}

type Stream interface {
	Recv() (Event, error)
	Close() error
}

type Client interface {
	Stream(ctx context.Context, request api.Request) (Stream, error)
}
