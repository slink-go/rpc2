package rpc2

import (
	"bufio"
	"github.com/slink-go/logging"
	"github.com/slink-go/rpc2/codec"
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

func (s *CustomRpcServer) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Warning("rpc.Accept: %s", err.Error())
			return
		}
		go s.ServeConn(conn)
	}
}
func (s *CustomRpcServer) ServeConn(conn io.ReadWriteCloser) {
	cdc := codec.GetServerCodec(bufio.NewWriter(conn), conn, s.cryptoKey)
	s.svr.ServeCodec(cdc)
}
func (s *CustomRpcServer) RegisterName(name string, service any) error {
	return s.svr.RegisterName(name, service)
}
