package rpc2

import (
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
)

type ServerOption func(*CustomRpcServer)

func ServerWithCryptoKey(value []byte) ServerOption {
	return func(s *CustomRpcServer) {
		s.cryptoKey = value
	}
}

func ServerWithAddress(value string) ServerOption {
	return func(s *CustomRpcServer) {
		s.address = value
	}
}

func ServerWithPort(value int) ServerOption {
	return func(s *CustomRpcServer) {
		s.port = value
	}
}

func ServerWithHandler(value ServerHandler) ServerOption {
	return func(s *CustomRpcServer) {
		s.handler = value
	}
}

func ServerWithLogger(value logging.Logger) ServerOption {
	return func(s *CustomRpcServer) {
		s.logger = value
	}
}

func ServerWithMiddleware(value rpc.MiddlewareFunc) ServerOption {
	return func(s *CustomRpcServer) {
		s.svr.Use(value)
	}
}
