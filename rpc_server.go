package rpc2

import (
	"bufio"
	"context"
	"errors"
	"go.slink.ws/logging"
	"go.slink.ws/rpc2/codec"
	"io"
	"net"
	"net/rpc"
)

type CustomRpcServer struct {
	logger    logging.Logger
	svr       *rpc.Server
	cryptoKey []byte
}

func NewRpcServer(opts ...RpcOption) *CustomRpcServer {
	cfg := DefaultRpcConfig()
	for _, opt := range opts {
		if opt != nil {
			opt.Apply(cfg)
		}
	}
	server := &CustomRpcServer{
		logger:    logging.GetLogger("rpc-server"),
		svr:       rpc.NewServer(),
		cryptoKey: cfg.Key,
	}
	return server
}

func (s *CustomRpcServer) Accept(ctx context.Context, listener net.Listener) {
	for {
		connChn := make(chan net.Conn)
		go s.waitForClient(connChn, listener)
		select {
		case <-ctx.Done():
			return
		case conn := <-connChn:
			go rpc.ServeConn(conn)
		}
	}
}
func (s *CustomRpcServer) ServeConn(conn io.ReadWriteCloser) {
	cdc := codec.GetServerCodec(bufio.NewWriter(conn), conn, s.cryptoKey)
	s.svr.ServeCodec(cdc)
}
func (s *CustomRpcServer) RegisterName(name string, service any) error {
	return s.svr.RegisterName(name, service)
}

func (s *CustomRpcServer) waitForClient(connChn chan net.Conn, listener net.Listener) {
	defer close(connChn)
	conn, err := listener.Accept()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			s.logger.Error("rpc.Accept: failed to accept client connection: %s", err)
		}
		return
	}
	connChn <- conn
}
