// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import (
	"bytes"
	"context"
	"encoding/gob"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.slink.ws/rpc"
	v2 "go.slink.ws/rpc2/v2"
)

// TestMain прогревает глобальный реестр типов encoding/gob до старта тестов:
// Encoder пишет idToTypeSlice под typeLock, а error-путь Decoder (undefined
// type — возникает, когда legacy gob-кодек получает JSON-байты при fallback)
// читает реестр без блокировки. При одновременной первой регистрации типа
// race detector фиксирует гонку stdlib. После прогрева новых типов в этом
// пакете не появляется — реестр стабилен.
func TestMain(m *testing.M) {
	var reqBuf bytes.Buffer
	if err := gob.NewEncoder(&reqBuf).Encode(&rpc.Request{}); err != nil {
		panic(err)
	}
	var respBuf bytes.Buffer
	if err := gob.NewEncoder(&respBuf).Encode(&rpc.Response{}); err != nil {
		panic(err)
	}
	var req rpc.Request
	if err := gob.NewDecoder(&reqBuf).Decode(&req); err != nil {
		panic(err)
	}
	var resp rpc.Response
	if err := gob.NewDecoder(&respBuf).Decode(&resp); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// region - test service

type integrationService int

func (integrationService) Hello(_ context.Context, name string, msg *string) error {
	*msg = "Hello, " + name + "!"
	return nil
}

// endregion
// region - helpers

type integrationClient struct {
	*CustomRpcClient
}

func (c *integrationClient) Hello(ctx context.Context, name string) (string, error) {
	var msg string
	err := c.Call(rpc.ContextWithHeaders(ctx, rpc.Header{}.Set("x-test-header", "v3")), "TestServer.Hello", name, &msg)
	return msg, err
}

func startIntegrationServer(t *testing.T, port int, opts ...ServerOption) {
	t.Helper()
	full := append([]ServerOption{
		ServerWithAddress("127.0.0.1"),
		ServerWithPort(port),
	}, opts...)
	svr := NewRpcServer(full...)
	require.NoError(t, svr.RegisterName("TestServer", new(integrationService)))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Accept(ctx) }()
	time.Sleep(50 * time.Millisecond)
}

func startV2Server(t *testing.T, port int, opts ...v2.ServerOption) {
	t.Helper()
	full := append([]v2.ServerOption{
		v2.ServerWithAddress("127.0.0.1"),
		v2.ServerWithPort(port),
	}, opts...)
	svr := v2.NewRpcServer(full...)
	require.NoError(t, svr.RegisterName("TestServer", new(integrationService)))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Accept(ctx) }()
	time.Sleep(50 * time.Millisecond)
}

// endregion
// region - sniff matrix

// TestIntegrationJSONFullCycle — v3 клиент (auto) ↔ v3 сервер без ключа: JSON-путь.
func TestIntegrationJSONFullCycle(t *testing.T) {
	t.Parallel()
	startIntegrationServer(t, 2356)

	c := NewRpcClient(ClientWithAddress("127.0.0.1"), ClientWithPort(2356))
	client := integrationClient{CustomRpcClient: c}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rsp, err := client.Hello(ctx, "JSON")
	require.NoError(t, err)
	assert.Equal(t, "Hello, JSON!", rsp)

	// второй вызов — поток синхронизирован (переустановка декодера)
	rsp, err = client.Hello(ctx, "JSON2")
	require.NoError(t, err)
	assert.Equal(t, "Hello, JSON2!", rsp)
}

// TestIntegrationCryptoFullCycle — ключ на обеих сторонах: clear-заголовок + CRYPT-тело.
func TestIntegrationCryptoFullCycle(t *testing.T) {
	t.Parallel()
	key := []byte("0123456789ABCDEF")
	startIntegrationServer(t, 2357, ServerWithCryptoKey(key))

	c := NewRpcClient(
		ClientWithAddress("127.0.0.1"),
		ClientWithPort(2357),
		ClientWithCryptoKey(key),
	)
	client := integrationClient{CustomRpcClient: c}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rsp, err := client.Hello(ctx, "Crypto")
	require.NoError(t, err)
	assert.Equal(t, "Hello, Crypto!", rsp)

	rsp, err = client.Hello(ctx, "Crypto2")
	require.NoError(t, err)
	assert.Equal(t, "Hello, Crypto2!", rsp)
}

// TestIntegrationV3ClientToV2Server — auto-клиент попадает на legacy-сервер:
// первый JSON-вызов падает по EOF, после fallback повтор на gob — успех.
func TestIntegrationV3ClientToV2Server(t *testing.T) {
	t.Parallel()
	startV2Server(t, 2358)

	c := NewRpcClient(ClientWithAddress("127.0.0.1"), ClientWithPort(2358))
	client := integrationClient{CustomRpcClient: c}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rsp, err := client.Hello(ctx, "Fallback")
	require.NoError(t, err)
	assert.Equal(t, "Hello, Fallback!", rsp)
}

// TestIntegrationV2ClientToV3Server — legacy-клиент (всегда gob) ↔ v3-сервер:
// sniff даёт legacy-ветку v2-кодека.
func TestIntegrationV2ClientToV3Server(t *testing.T) {
	t.Parallel()
	startIntegrationServer(t, 2359)

	c := v2.NewRpcClient(
		v2.ClientWithAddress("127.0.0.1"),
		v2.ClientWithPort(2359),
	)
	type v2Client struct{ *v2.CustomRpcClient }
	// обёртка с тем же контрактом, что и integrationClient
	cl := v2Client{CustomRpcClient: c}
	var msg string
	err := cl.Call(context.Background(), "TestServer.Hello", "Legacy", &msg)
	require.NoError(t, err)
	assert.Equal(t, "Hello, Legacy!", msg)
}

// endregion
