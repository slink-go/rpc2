package proxy

import (
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.slink.ws/rpc"
	v2 "go.slink.ws/rpc2/v2"
	rpc2 "go.slink.ws/rpc2/v3"
)

// region - typed mailer-бэкенд (hikari-подобный: конкретные типы аргументов)

type SendReq struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

type SendRes struct {
	Accepted bool   `json:"accepted"`
	QueueID  string `json:"queue_id"`
}

// mailerBackend — имитация реального mailer-сервиса: сервер сам декодирует
// JSON-payload прокси в конкретные типы (двойной декод не нужен).
type mailerBackend struct {
	mu      sync.Mutex
	last    *SendReq
	headers rpc.Header
}

// SendTemplate — метод в духе mailer.SendTemplate.
func (b *mailerBackend) SendTemplate(ctx context.Context, req SendReq, res *SendRes) error {
	b.mu.Lock()
	b.last = &req
	b.headers = rpc.Header{}
	for k, vs := range rpc.ContextHeader(ctx) {
		b.headers[k] = append([]string(nil), vs...)
	}
	b.mu.Unlock()
	res.Accepted = req.To != ""
	res.QueueID = "q-1"
	return nil
}

// Slow — «долгий» метод для проверки таймаута.
func (b *mailerBackend) Slow(_ context.Context, _ struct{}, _ *struct{}) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

func (b *mailerBackend) snapshot() (*SendReq, rpc.Header) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.last, b.headers
}

// endregion
// region - bootstrap

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// startV3Backend поднимает rpc2/v3-сервер и возвращает Target для роутинга.
func startV3Backend(t *testing.T, name string, svc any) Target {
	t.Helper()
	port := freePort(t)
	svr := rpc2.NewRpcServer(rpc2.ServerWithAddress("127.0.0.1"), rpc2.ServerWithPort(port))
	require.NoError(t, svr.RegisterName(name, svc))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Accept(ctx) }()
	time.Sleep(50 * time.Millisecond)
	return Target{Addr: "127.0.0.1", Port: port, Transport: "json"}
}

// startV2GobBackend поднимает legacy gob-сервер (v2) — бэкенд без JSON.
func startV2GobBackend(t *testing.T, name string, svc any) Target {
	t.Helper()
	port := freePort(t)
	svr := v2.NewRpcServer(v2.ServerWithAddress("127.0.0.1"), v2.ServerWithPort(port))
	require.NoError(t, svr.RegisterName(name, svc))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Accept(ctx) }()
	time.Sleep(50 * time.Millisecond)
	return Target{Addr: "127.0.0.1", Port: port, Transport: "json"}
}

// startProxyFront регистрирует proxy.Service как rpc2-сервис и возвращает
// v3-клиент к фронту прокси.
func startProxyFront(t *testing.T, router Router, allow Allowlister, invOpts ...InvokerOption) *rpc2.CustomRpcClient {
	t.Helper()
	port := freePort(t)
	svr := rpc2.NewRpcServer(rpc2.ServerWithAddress("127.0.0.1"), rpc2.ServerWithPort(port))
	require.NoError(t, svr.RegisterName(ServiceName, New(router, allow, NewRPCInvoker(invOpts...))))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Accept(ctx) }()
	time.Sleep(50 * time.Millisecond)

	client := rpc2.NewRpcClient(
		rpc2.ClientWithAddress("127.0.0.1"),
		rpc2.ClientWithPort(port),
		rpc2.ClientWithTransport("json"),
	)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// proxyCall — вызов proxy.Call с фронта (путь typed-пользователя
// generic-путём, как hikari).
func proxyCall(t *testing.T, client *rpc2.CustomRpcClient, ctx context.Context, req GenericRequest) *GenericResponse {
	t.Helper()
	var res GenericResponse
	require.NoError(t, client.Call(ctx, ServiceName+".Call", req, &res))
	return &res
}

// mustRouter — StaticRouter или паника теста.
func mustRouter(t *testing.T, targets map[string]Target) Router {
	t.Helper()
	r, err := NewStaticRouter(targets)
	require.NoError(t, err)
	return r
}

// endregion

// TestE2E_SendTemplateGenericPath — позитив T2.8: typed-бэкенд получает
// JSON-payload прокси байт-в-байт и сам декодирует его в тип аргумента.
func TestE2E_SendTemplateGenericPath(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV3Backend(t, "mailer", backend)
	client := startProxyFront(t, mustRouter(t, map[string]Target{"mailer": tgt}), nil)

	res := proxyCall(t, client, context.Background(), GenericRequest{
		Method:  "mailer.SendTemplate",
		Payload: []byte(`{"to":"user@example.com","subject":"hello"}`),
	})
	require.Nil(t, res.Error)
	assert.JSONEq(t, `{"accepted":true,"queue_id":"q-1"}`, string(res.Payload))

	last, _ := backend.snapshot()
	require.NotNil(t, last)
	assert.Equal(t, "user@example.com", last.To, "сервер обязан декодировать JSON в SendReq")
	assert.Equal(t, "hello", last.Subject)
}

// TestE2E_IdentityHeadersWhitelist — T2.9 (security): из ctx фронта и
// req.Metadata наружу уходят только whitelisted-ключи; ctx (auth) важнее
// metadata.
func TestE2E_IdentityHeadersWhitelist(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV3Backend(t, "mailer", backend)
	client := startProxyFront(t, mustRouter(t, map[string]Target{"mailer": tgt}), nil)

	ctx := rpc.ContextWithHeaders(context.Background(), rpc.Header{}.
		Set("X-User-Id", "ctx-user").
		Set("X-Sneaky", "drop-me"))
	res := proxyCall(t, client, ctx, GenericRequest{
		Method:  "mailer.SendTemplate",
		Payload: []byte(`{"to":"a@b.c"}`),
		Metadata: map[string]string{
			"X-User-Id": "md-user",
			"X-Groups":  "g1,g2",
			"X-Evil":    "drop-me-too",
		},
	})
	require.Nil(t, res.Error)

	_, headers := backend.snapshot()
	assert.Equal(t, "ctx-user", headers.Get("X-User-Id"), "ctx (auth-middleware) перекрывает metadata")
	assert.Equal(t, "g1,g2", headers.Get("X-Groups"))
	assert.Empty(t, headers.Get("X-Sneaky"))
	assert.Empty(t, headers.Values("X-Evil"))
}

// TestE2E_AllowlistDeniesMethod — T2.9: непустой allowlist запрещает метод
// (бэкенд не вызывается); пустой/отсутствующий — allow-all (позитив выше).
func TestE2E_AllowlistDeniesMethod(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV3Backend(t, "mailer", backend)
	allow, err := NewAllowlist([]string{"mailer.SendMail"}, PolicyDefault)
	require.NoError(t, err)
	client := startProxyFront(t, mustRouter(t, map[string]Target{"mailer": tgt}), allow)

	res := proxyCall(t, client, context.Background(), GenericRequest{
		Method:  "mailer.SendTemplate",
		Payload: []byte(`{"to":"a@b.c"}`),
	})
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeAllowlist, res.Error.Code)

	last, _ := backend.snapshot()
	assert.Nil(t, last, "бэкенд не должен вызываться при отказе allowlist")
}

// TestE2E_GobOnlyBackendNoJSONError — T2.9: legacy gob-бэкенд (v2) даёт
// CodeNoJSON с пояснением, а не молчаливый обрыв.
func TestE2E_GobOnlyBackendNoJSONError(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV2GobBackend(t, "mailer", backend)
	client := startProxyFront(t,
		mustRouter(t, map[string]Target{"mailer": tgt}), nil,
		InvokerWithCallTimeout(1*time.Second),
	)

	res := proxyCall(t, client, context.Background(), GenericRequest{
		Method:  "mailer.SendTemplate",
		Payload: []byte(`{"to":"a@b.c"}`),
	})
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeNoJSON, res.Error.Code, "got: %+v", res.Error)
	assert.Contains(t, res.Error.Message, "does not support JSON")
	assert.Equal(t, "127.0.0.1:"+strconv.Itoa(tgt.Port), res.Error.Target)
}

// TestE2E_PayloadCap — T2.9: превышение payload cap — структурированная
// ошибка CodePayload, бэкенд не вызывается.
func TestE2E_PayloadCap(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV3Backend(t, "mailer", backend)
	client := startProxyFront(t,
		mustRouter(t, map[string]Target{"mailer": tgt}), nil,
		InvokerWithMaxPayload(16),
	)

	res := proxyCall(t, client, context.Background(), GenericRequest{
		Method:  "mailer.SendTemplate",
		Payload: []byte(`{"to":"very-long-address@example.com"}`),
	})
	require.NotNil(t, res.Error)
	assert.Equal(t, CodePayload, res.Error.Code)
	assert.Contains(t, res.Error.Message, "request payload")

	last, _ := backend.snapshot()
	assert.Nil(t, last)
}

// TestE2E_CallTimeout — T2.9: долгий бэкенд-вызов упирается в таймаут
// invoker'а → CodeTransport c пояснением.
func TestE2E_CallTimeout(t *testing.T) {
	t.Parallel()
	backend := &mailerBackend{}
	tgt := startV3Backend(t, "mailer", backend)
	client := startProxyFront(t,
		mustRouter(t, map[string]Target{"mailer": tgt}), nil,
		InvokerWithCallTimeout(50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res := proxyCall(t, client, ctx, GenericRequest{
		Method:  "mailer.Slow",
		Payload: []byte(`{}`),
	})
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeTransport, res.Error.Code)
	assert.Contains(t, res.Error.Message, "call timeout")
}
