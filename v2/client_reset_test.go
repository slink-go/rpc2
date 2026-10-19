// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EnvelopeReply повторяет контракт сервисов: бизнес-ошибка приходит в теле,
// Go-ошибка остаётся nil.
type EnvelopeReply struct {
	Error string
	Data  string
}

type resetProbe struct {
	fail atomic.Bool
}

func (p *resetProbe) Ping(_ context.Context, in string, out *string) error {
	if p.fail.Load() {
		return errors.New("boom")
	}
	*out = in
	return nil
}

func (p *resetProbe) Biz(_ context.Context, in string, out *EnvelopeReply) error {
	out.Error = "bad input"
	out.Data = in
	return nil
}

func startResetEnv(t *testing.T) (*resetProbe, *tcpProxy, *CustomRpcClient) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	port := freePort(t)
	svr := NewRpcServer(
		ServerWithAddress("127.0.0.1"),
		ServerWithPort(port),
		ServerWithCryptoKey([]byte(testCryptoKey)),
	)
	probe := &resetProbe{}
	require.NoError(t, svr.RegisterName("Probe", probe))
	go func() { _ = svr.Accept(ctx) }()

	serverAddr := "127.0.0.1:" + strconv.Itoa(port)
	waitDial(t, serverAddr)

	proxy := newTCPProxy(t, serverAddr)
	client := NewRpcClient(
		ClientWithAddress("127.0.0.1"),
		ClientWithPort(proxy.addr().(*net.TCPAddr).Port),
		ClientWithCryptoKey([]byte(testCryptoKey)),
	)
	t.Cleanup(func() { _ = client.Close() })
	return probe, proxy, client
}

// Ошибка вызова (rpc.ServerError) считается инфраструктурной: соединение
// сбрасывается, следующий вызов идёт по новому соединению.
func TestCallResetsConnectionOnServerError(t *testing.T) {
	probe, proxy, client := startResetEnv(t)
	ctx := context.Background()

	probe.fail.Store(true)
	var out string
	err := client.Call(ctx, "Probe.Ping", "x", &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.EqualValues(t, 1, proxy.totalConns())

	probe.fail.Store(false)
	require.NoError(t, client.Call(ctx, "Probe.Ping", "ok", &out))
	assert.Equal(t, "ok", out)
	assert.EqualValues(t, 2, proxy.totalConns(), "клиент обязан переподключиться")
}

// Бизнес-ошибка в теле ответа не рвёт соединение.
func TestCallKeepsConnectionOnBodyError(t *testing.T) {
	_, proxy, client := startResetEnv(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		var out EnvelopeReply
		require.NoError(t, client.Call(ctx, "Probe.Biz", "x", &out))
		assert.Equal(t, "bad input", out.Error)
	}
	assert.EqualValues(t, 1, proxy.totalConns(), "бизнес-ошибка не должна переподключать")
}
