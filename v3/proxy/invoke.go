package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"

	"go.slink.ws/logging"
	"go.slink.ws/rpc"
	rpc2 "go.slink.ws/rpc2/v3"
)

// identityHeaderKeys — единственные заголовки, пробрасываемые наружу к
// целевому сервису. Всё остальное (metadata и заголовки входа) отбрасывается.
var identityHeaderKeys = []string{"X-User-Id", "X-Tenant-Id", "X-Groups", "X-Request-Id"}

// rpcClient — контракт клиента, достаточного для вызова (подмена в тестах).
type rpcClient interface {
	Call(ctx context.Context, method string, args, reply any) error
	Close() error
}

// defaultCallTimeout — дефолтный таймаут одного вызова.
const defaultCallTimeout = 30 * time.Second

// RPCInvoker — выполнение вызова целевого сервиса через rpc2/v3 (JSON).
// Реализует интерфейс Invoker.
type RPCInvoker struct {
	logger      logging.Logger
	maxPayload  int64
	callTimeout time.Duration
	limitCh     chan struct{}
	newClient   func(Target) (rpcClient, error)
}

// InvokerOption — опция RPCInvoker.
type InvokerOption func(*RPCInvoker)

// InvokerWithLogger задаёт логгер.
func InvokerWithLogger(value logging.Logger) InvokerOption {
	return func(i *RPCInvoker) { i.logger = value }
}

// InvokerWithMaxPayload ограничивает размер payload в обе стороны
// (≤0 — дефолт JSON-кодека, 4MB).
func InvokerWithMaxPayload(value int64) InvokerOption {
	return func(i *RPCInvoker) { i.maxPayload = value }
}

// InvokerWithCallTimeout задаёт таймаут одного вызова (≤0 — без изменения
// дефолта).
func InvokerWithCallTimeout(value time.Duration) InvokerOption {
	return func(i *RPCInvoker) { i.callTimeout = value }
}

// InvokerWithMaxConcurrent ограничивает число одновременных вызовов
// (≤0 — без ограничения).
func InvokerWithMaxConcurrent(value int) InvokerOption {
	return func(i *RPCInvoker) {
		if value > 0 {
			i.limitCh = make(chan struct{}, value)
		}
	}
}

// InvokerWithClientFactory подменяет фабрику клиентов (тесты).
func InvokerWithClientFactory(value func(Target) (rpcClient, error)) InvokerOption {
	return func(i *RPCInvoker) { i.newClient = value }
}

// NewRPCInvoker собирает RPCInvoker. По умолчанию: payload cap — 4MB
// (дефолт JSON-кодека), таймаут вызова — 30s, лимита конкуренции нет.
func NewRPCInvoker(opts ...InvokerOption) *RPCInvoker {
	inv := &RPCInvoker{
		logger:      logging.GetLogger("proxy-invoker"),
		maxPayload:  0,
		callTimeout: defaultCallTimeout,
		newClient:   defaultClientFactory,
	}
	for _, opt := range opts {
		opt(inv)
	}
	return inv
}

// Invoke — проброс идентичности (whitelist-заголовки), вызов target-сервиса
// JSON-транспортом, маппинг ошибок.
//
// Payload передаётся как json.RawMessage — байт-в-байт, без двойного
// кодирования; целевой jRPC-сервис сам декодирует его в тип аргумента метода.
func (inv *RPCInvoker) Invoke(ctx context.Context, target Target, method string, payload []byte, md map[string]string) ([]byte, *CallError) {
	tgtStr := targetString(target)

	if err := ctx.Err(); err != nil {
		return nil, &CallError{Code: CodeTransport, Message: err.Error(), Target: tgtStr}
	}

	client, err := inv.newClient(target)
	if err != nil {
		return nil, &CallError{Code: CodeTransport, Message: "proxy: client init: " + err.Error(), Target: tgtStr}
	}
	defer func() { _ = client.Close() }()

	callCtx, cancel := context.WithTimeout(ctx, inv.callTimeout)
	defer cancel()
	callCtx = rpc.ContextWithHeaders(callCtx, identityHeaders(ctx, md))

	var reply json.RawMessage
	if err = client.Call(callCtx, method, json.RawMessage(payload), &reply); err != nil {
		return nil, mapCallError(err, tgtStr)
	}

	out := make([]byte, len(reply))
	copy(out, reply)
	return out, nil
}

// identityHeaders собирает whitelist-заголовки: значения из req.Metadata,
// перекрытые значениями из входного ctx (auth-middleware — доверенный
// источник, приоритет выше). Не-whitelisted ключи отбрасываются.
func identityHeaders(ctx context.Context, md map[string]string) rpc.Header {
	h := rpc.Header{}
	for _, k := range identityHeaderKeys {
		if v := md[k]; v != "" {
			h.Set(k, v)
		}
	}
	inbound := rpc.ContextHeader(ctx)
	for _, k := range identityHeaderKeys {
		if vs := inbound.Values(k); len(vs) > 0 {
			h[k] = vs
		}
	}
	return h
}

// targetString — "addr:port" для CallError.Target и логов.
func targetString(t Target) string {
	return net.JoinHostPort(t.Addr, strconv.Itoa(t.Port))
}

// mapCallError классифицирует ошибку вызова (T2.6 уточняет коды).
func mapCallError(err error, target string) *CallError {
	var srvErr rpc.ServerError
	if errors.As(err, &srvErr) {
		return &CallError{Code: CodeBackend, Message: srvErr.Error(), Target: target}
	}
	return &CallError{Code: CodeTransport, Message: err.Error(), Target: target}
}

// defaultClientFactory создаёт rpc2/v3-клиент. Транспорт нормализуется:
// "" и "auto" → "json": прокси требует JSON-бэкенд (auto-фолбэк в gob
// разрушил бы смысл RawMessage-payload).
func defaultClientFactory(tgt Target) (rpcClient, error) {
	transport := tgt.Transport
	if transport == "" || transport == "auto" {
		transport = "json"
	}
	opts := []rpc2.ClientOption{
		rpc2.ClientWithAddress(tgt.Addr),
		rpc2.ClientWithPort(tgt.Port),
		rpc2.ClientWithTransport(transport),
	}
	if len(tgt.Secret) > 0 {
		opts = append(opts, rpc2.ClientWithCryptoKey(tgt.Secret))
	}
	return rpc2.NewRpcClient(opts...), nil
}
