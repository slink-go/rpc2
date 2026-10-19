// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import (
	"bufio"
	"context"
	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	"go.slink.ws/rpc2/v2/codec"
	"net"
	"strconv"
	"sync"
	"time"
)

type CustomRpcClient struct {
	address     string
	port        int
	cryptoKey   []byte
	middlewares []ClientContextMiddleware
	logger      logging.Logger

	mu sync.Mutex
	cl *rpc.Client
}

func NewRpcClient(opts ...ClientOption) *CustomRpcClient {
	client := &CustomRpcClient{
		logger:  logging.GetLogger("rpc-client"),
		port:    2233,
		address: "127.0.0.1",
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

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

	if err = cl.Call(ctx, method, args, reply); err != nil {
		// Бизнес-ошибки сервисы возвращают в теле ответа. Любая ошибка
		// транспорта означает, что состояние соединения не определено:
		// сбрасываем его, следующий вызов переподключится.
		c.logger.Warning("rpc client %s:%d: resetting connection after %s: %v", c.address, c.port, method, err)
		c.invalidate()
		return err
	}
	if errCtx := ctx.Err(); errCtx != nil {
		return errCtx
	}
	return nil
}

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

	codec, err := codec.NewClientCodec(bufio.NewWriter(conn), conn, c.cryptoKey)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	cl := rpc.NewClientWithCodec(codec)
	c.cl = cl
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
