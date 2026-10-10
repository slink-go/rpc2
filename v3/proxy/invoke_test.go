package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.slink.ws/rpc"
)

// fakeClient записывает вызов и возвращает заданную ошибку/ответ.
type fakeClient struct {
	callErr  error
	reply    string
	gotArgs  json.RawMessage
	gotMeth  string
	gotMD    rpc.Header
	closed   bool
	callWait func(ctx context.Context)
}

func (f *fakeClient) Call(ctx context.Context, method string, args, reply any) error {
	f.gotMeth = method
	if raw, ok := args.(json.RawMessage); ok {
		f.gotArgs = append(json.RawMessage(nil), raw...)
	}
	f.gotMD = rpc.ContextHeader(ctx)
	if f.callWait != nil {
		f.callWait(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.callErr != nil {
		return f.callErr
	}
	return json.Unmarshal([]byte(f.reply), reply)
}

func (f *fakeClient) Close() error {
	f.closed = true
	return nil
}

func newTestInvoker(fc *fakeClient, opts ...InvokerOption) *RPCInvoker {
	all := append([]InvokerOption{
		InvokerWithClientFactory(func(Target) (rpcClient, error) { return fc, nil }),
	}, opts...)
	return NewRPCInvoker(all...)
}

var testTarget = Target{Addr: "svc", Port: 1234, Transport: "json"}

func TestInvokeVerbatimPayloadAndMethod(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{reply: `{"ok":true}`}
	inv := newTestInvoker(fc)

	payload := []byte(`{"n":1,"nested":{"a":[1,2]}}`)
	out, cerr := inv.Invoke(context.Background(), testTarget, "miki.Get", payload, nil)
	require.Nil(t, cerr)
	assert.Equal(t, "miki.Get", fc.gotMeth)
	assert.Equal(t, payload, []byte(fc.gotArgs), "payload обязан пробрасываться байт-в-байт")
	assert.JSONEq(t, `{"ok":true}`, string(out))
	assert.True(t, fc.closed, "клиент закрывается после вызова")
}

func TestInvokeIdentityHeadersWhitelistAndPrecedence(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{reply: `{}`}
	inv := newTestInvoker(fc)

	inbound := rpc.Header{}
	inbound.Set("X-User-Id", "from-ctx")
	inbound.Set("X-Tenant-Id", "t1")
	inbound.Set("X-Sneaky", "drop-ctx")
	ctx := rpc.ContextWithHeaders(context.Background(), inbound)

	md := map[string]string{
		"X-User-Id":  "from-md",
		"X-Groups":   "g1,g2",
		"X-Sneaky":   "drop-md",
		"Extra-Key":  "drop",
		"X-Groups-X": "drop",
	}
	_, cerr := inv.Invoke(ctx, testTarget, "miki.Get", []byte(`{}`), md)
	require.Nil(t, cerr)

	h := fc.gotMD
	assert.Equal(t, "from-ctx", h.Get("X-User-Id"), "ctx (auth-middleware) перекрывает metadata")
	assert.Equal(t, "t1", h.Get("X-Tenant-Id"))
	assert.Equal(t, "g1,g2", h.Get("X-Groups"), "whitelist-ключ из metadata проходит")
	assert.Empty(t, h.Get("X-Sneaky"))
	assert.Empty(t, h.Values("Extra-Key"))
	assert.Empty(t, h.Values("X-Groups-X"))
}

func TestInvokeErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"ServerError → backend", rpc.ServerError("boom"), CodeBackend},
		{"прочая ошибка → transport", errors.New("dial refused"), CodeTransport},
		{"context deadline → transport", context.DeadlineExceeded, CodeTransport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fc := &fakeClient{callErr: tc.err}
			inv := newTestInvoker(fc)
			_, cerr := inv.Invoke(context.Background(), testTarget, "miki.Get", []byte(`{}`), nil)
			require.NotNil(t, cerr)
			assert.Equal(t, tc.code, cerr.Code)
			assert.Equal(t, "svc:1234", cerr.Target)
			assert.Contains(t, cerr.Message, tc.err.Error())
		})
	}
}

func TestInvokeContextAlreadyDone(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{reply: `{}`}
	inv := newTestInvoker(fc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, cerr := inv.Invoke(ctx, testTarget, "miki.Get", []byte(`{}`), nil)
	require.NotNil(t, cerr)
	assert.Equal(t, CodeTransport, cerr.Code)
	assert.Nil(t, fc.callErr, "вызов не выполняется при уже отменённом ctx")
	assert.Empty(t, fc.gotMeth)
}

func TestInvokeFactoryError(t *testing.T) {
	t.Parallel()
	inv := NewRPCInvoker(InvokerWithClientFactory(func(Target) (rpcClient, error) {
		return nil, errors.New("no route")
	}))
	_, cerr := inv.Invoke(context.Background(), testTarget, "miki.Get", []byte(`{}`), nil)
	require.NotNil(t, cerr)
	assert.Equal(t, CodeTransport, cerr.Code)
	assert.Contains(t, cerr.Message, "no route")
}

func TestDefaultClientFactoryTransportNormalization(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"", "auto"} {
		c, err := defaultClientFactory(Target{Addr: "h", Port: 1, Transport: transport})
		require.NoError(t, err)
		require.NotNil(t, c)
		_ = c.Close()
	}
}

func TestInvokeTimeoutFromContext(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{reply: `{}`, callWait: func(ctx context.Context) {
		<-ctx.Done()
	}}
	inv := newTestInvoker(fc, InvokerWithCallTimeout(20*time.Millisecond))

	_, cerr := inv.Invoke(context.Background(), testTarget, "miki.Get", []byte(`{}`), nil)
	require.NotNil(t, cerr)
	assert.Equal(t, CodeTransport, cerr.Code)
}
