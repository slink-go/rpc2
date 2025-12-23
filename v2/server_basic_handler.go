package rpc2

import (
	"context"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
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
