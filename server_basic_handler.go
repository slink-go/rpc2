package rpc2

import (
	"context"
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
)

type BasicServerHandler struct {
	logger logging.Logger
	svr    *rpc.Server
}

func NewBasicServerHandler(server *rpc.Server) *BasicServerHandler {
	return &BasicServerHandler{
		logger: logging.GetLogger("basic-handler"),
		svr:    server,
	}
}

func (h *BasicServerHandler) Handle(ctx context.Context, codec rpc.ServerCodec) {
	_ = h.svr.ServeRequest(ctx, codec) // skip error handling in basic handler
}
