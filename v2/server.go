package rpc2

import (
	"bufio"
	"context"
	"errors"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"go.slink.ws/rpc2/v2/codec"
	"net"
	"strconv"
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
	addr := net.JoinHostPort(s.address, strconv.Itoa(s.port))
	addy, err := net.ResolveTCPAddr(tcp, addr)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP(tcp, addy)
	if err != nil {
		return err
	}
	s.logger.Info("rpc server listening on %s", addr)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return ctx.Err()
			}
			s.logger.Error("rpc.Accept: failed to accept client connection: %s", err)
			continue
		}
		_ = conn.SetKeepAlive(true)
		_ = conn.SetNoDelay(true)
		go s.ServeConn(ctx, conn)
	}
}

func (s *CustomRpcServer) ServeConn(ctx context.Context, conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("rpc: panic serving connection from %s: %v", conn.RemoteAddr(), r)
		}
		_ = conn.Close()
	}()

	cdc, err := codec.NewServerCodec(bufio.NewWriter(conn), conn, s.cryptoKey)
	if err != nil {
		s.logger.Error("rpc: create server codec: %s", err)
		return
	}
	defer func() { _ = cdc.Close() }()

	cctx := ctx
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		cctx = context.WithValue(ctx, remoteIpHeader, addr.IP.String())
	}
	s.handler.Handle(cctx, cdc)
}

func (s *CustomRpcServer) RegisterName(name string, service any) error {
	return s.svr.RegisterName(name, service)
}
