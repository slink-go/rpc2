package rpc2

import (
	"go.slink.ws/logging"
	"net/rpc"
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

func (h *BasicServerHandler) Handle(codec rpc.ServerCodec) {
	_ = h.svr.ServeRequest(codec) // skip error handling in basic handler
}
