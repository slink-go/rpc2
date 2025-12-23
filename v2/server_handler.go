package rpc2

import (
	"context"
	"go.slink.ws/rpc"
)

type ServerHandler interface {
	Handle(ctx context.Context, codec rpc.ServerCodec)
}
