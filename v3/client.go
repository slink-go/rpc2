package rpc2

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"go.slink.ws/rpc2/v3/codec"
)

// transportKind — режим транспорта клиента rpc2/v3.
type transportKind int

const (
	// transportAuto — по умолчанию JSON; при ошибке не-ServerError на первом
	// вызове — однократный фолбэк в legacy (gob) и sticky-режим дальше.
	transportAuto transportKind = iota
	// transportJSON — всегда JSON (без фолбэка).
	transportJSON
	// transportLegacy — всегда gob/crypto как в v2.
	transportLegacy
)

// CustomRpcClient — клиент rpc2/v3 с auto-фолбэком на legacy-серверы.
type CustomRpcClient struct {
	address     string
	port        int
	cryptoKey   []byte
	maxJSONBody int64
	transport   transportKind
	middlewares []ClientContextMiddleware
	logger      logging.Logger

	mu              sync.Mutex
	cl              *rpc.Client
	activeTransport transportKind
	jsonFallback    bool
}

// NewRpcClient собирает клиент по опциям (по умолчанию transport=auto).
func NewRpcClient(opts ...ClientOption) *CustomRpcClient {
	client := &CustomRpcClient{
		logger:    logging.GetLogger("rpc-client"),
		port:      2233,
		address:   "127.0.0.1",
		transport: transportAuto,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// Call выполняет RPC-вызов; при conn-ошибке на JSON-транспорте в auto-режиме
// однократно повторяет вызов на legacy-соединении.
func (c *CustomRpcClient) Call(ctx context.Context, method string, args any, reply any) error {
	for _, mw := range c.middlewares {
		ctx = mw(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	cl, err := c.getClient(ctx)
	if err != nil {
		return err
	}

	err = cl.Call(ctx, method, args, reply)
	if err == nil {
		return ctx.Err()
	}
	if !isConnError(err) {
		return err
	}
	c.mu.Lock()
	wasJSON := c.activeTransport == transportJSON
	alreadyFallback := c.jsonFallback
	c.jsonFallback = true
	c.mu.Unlock()
	c.invalidate()

	// автофолбэк: один повтор на legacy-соединении
	if c.transport == transportAuto && wasJSON && !alreadyFallback {
		c.logger.Info("rpc: json transport failed, falling back to legacy: %s", method)
		cl, err = c.getClient(ctx)
		if err != nil {
			return err
		}
		err = cl.Call(ctx, method, args, reply)
		if err == nil {
			return ctx.Err()
		}
		if isConnError(err) {
			c.invalidate()
		}
		return err
	}
	return err
}

// Close закрывает соединение.
func (c *CustomRpcClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cl == nil {
		return nil
	}
	err := c.cl.Close()
	c.cl = nil
	return err
}

// effectiveTransport — какой кодек использовать для нового соединения.
func (c *CustomRpcClient) effectiveTransport() transportKind {
	switch c.transport {
	case transportJSON:
		return transportJSON
	case transportLegacy:
		return transportLegacy
	default:
		if c.jsonFallback {
			return transportLegacy
		}
		return transportJSON
	}
}

func (c *CustomRpcClient) getClient(ctx context.Context) (*rpc.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cl != nil {
		return c.cl, nil
	}

	conn, err := (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext(ctx, tcp, net.JoinHostPort(c.address, strconv.Itoa(c.port)))
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	kind := c.effectiveTransport()
	cdc, err := codec.NewClientCodecV3(bufio.NewWriter(conn), conn, c.cryptoKey, c.maxJSONBody, kind == transportJSON)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	cl := rpc.NewClientWithCodec(cdc)
	c.cl = cl
	c.activeTransport = kind
	return cl, nil
}

func (c *CustomRpcClient) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cl != nil {
		_ = c.cl.Close()
		c.cl = nil
	}
}

func isConnError(err error) bool {
	if err == nil {
		return false
	}
	var se rpc.ServerError
	if errors.As(err, &se) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return errors.Is(err, rpc.ErrShutdown) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}
