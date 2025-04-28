package rpc2

import (
	"context"
	"github.com/renevo/rpc"
)

type ServerHandler interface {
	Handle(ctx context.Context, codec rpc.ServerCodec)
}
