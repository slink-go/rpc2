package rpc2

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/renevo/rpc"
	"go.slink.ws/logging"
	"go.slink.ws/rpc2/codec"
	"net"
)

const (
	tcp            = "tcp"
	remoteIpHeader = "CLIENT-IP-ADDRESS"
)

type CustomRpcServer struct {
	address   string
	port      int
	cryptoKey []byte
	svr       *rpc.Server
	handler   ServerHandler
	logger    logging.Logger
}

func NewRpcServer(opts ...ServerOption) *CustomRpcServer {
	svr := rpc.NewServer()
	server := &CustomRpcServer{
		logger:  logging.GetLogger("rpc-server"),
		port:    2233,
		address: "0.0.0.0",
		handler: NewBasicServerHandler(svr),
		svr:     svr,
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *CustomRpcServer) Accept(ctx context.Context) error {

	addr := fmt.Sprintf("%v:%v", s.address, s.port)

	addy, err := net.ResolveTCPAddr(tcp, addr)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP(tcp, addy)
	if err != nil {
		return err
	}

	for {
		connChn := make(chan net.Conn)
		go s.waitForClient(connChn, listener)
		select {
		case <-ctx.Done():
			_ = listener.Close()
			s.logger.Info("stopped rpc server")
			return ctx.Err()
		case conn := <-connChn:
			go s.ServeConn(ctx, conn)
		}
	}

}
func (s *CustomRpcServer) ServeConn(ctx context.Context, conn net.Conn) {
	cdc := codec.GetServerCodec(bufio.NewWriter(conn), conn, s.cryptoKey)
	defer func() { _ = cdc.Close() }()
	addr := conn.RemoteAddr().(*net.TCPAddr)
	cctx := context.WithValue(ctx, remoteIpHeader, addr.IP.String())
	s.handler.Handle(cctx, cdc)
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
