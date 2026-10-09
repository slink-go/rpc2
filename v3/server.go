package rpc2

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"

	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"go.slink.ws/rpc2/v3/codec"
)

const (
	tcp            = "tcp"
	remoteIpHeader = "CLIENT-IP-ADDRESS"
)

// ctxKey — собственный тип ключа контекста (избегает коллизий SA1029).
type ctxKey string

// ServerHandler — обработчик соединений (как в v2).
type ServerHandler interface {
	Handle(ctx context.Context, codec rpc.ServerCodec)
}

// BasicServerHandler отдаёт кодек в rpc.Server.ServeCodec.
type BasicServerHandler struct {
	logger logging.Logger
	svr    *rpc.Server
}

// NewBasicServerHandler создаёт обработчик поверх rpc.Server.
func NewBasicServerHandler(server *rpc.Server) *BasicServerHandler {
	return &BasicServerHandler{logger: logging.GetLogger("basic-handler"), svr: server}
}

// Handle обслуживает соединение через кодек.
func (h *BasicServerHandler) Handle(ctx context.Context, codec rpc.ServerCodec) {
	h.svr.ServeCodec(ctx, codec)
}

// CustomRpcServer — сервер rpc2/v3: sniff первого байта на каждое соединение,
// JSON-кодек для новых клиентов, legacy gob/crypto — байт-в-байт как в v2.
type CustomRpcServer struct {
	address     string
	port        int
	cryptoKey   []byte
	maxJSONBody int64
	svr         *rpc.Server
	handler     ServerHandler
	logger      logging.Logger
}

// NewRpcServer собирает сервер по опциям (по умолчанию auto-режим sniff).
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

// Accept слушает адрес и обслуживает каждое соединение в отдельной горутине.
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

// ServeConn выбирает кодек по первому байту потока (sniff в фабрике) и
// обслуживает соединение.
func (s *CustomRpcServer) ServeConn(ctx context.Context, conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("rpc: panic serving connection from %s: %v", conn.RemoteAddr(), r)
		}
		_ = conn.Close()
	}()

	cdc, err := codec.NewServerCodecV3(bufio.NewWriter(conn), conn, s.cryptoKey, s.maxJSONBody)
	if err != nil {
		s.logger.Error("rpc: create server codec: %s", err)
		return
	}
	defer func() { _ = cdc.Close() }()

	cctx := ctx
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		cctx = context.WithValue(ctx, ctxKey(remoteIpHeader), addr.IP.String())
	}
	s.handler.Handle(cctx, cdc)
}

// RegisterName регистрирует сервис под именем (namespace).
func (s *CustomRpcServer) RegisterName(name string, service any) error {
	return s.svr.RegisterName(name, service)
}
