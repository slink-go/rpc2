package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRouter struct {
	tgt    Target
	err    error
	got    []string
	called bool
}

func (f *fakeRouter) Resolve(service string) (Target, error) {
	f.called = true
	f.got = append(f.got, service)
	return f.tgt, f.err
}

type fakeAllow struct {
	err    error
	got    []string
	called bool
}

func (f *fakeAllow) Allowed(method string) error {
	f.called = true
	f.got = append(f.got, method)
	return f.err
}

type fakeInvoker struct {
	payload []byte
	cerr    *CallError
	got     struct {
		method  string
		payload []byte
		md      map[string]string
		tgt     Target
	}
	called bool
}

func (f *fakeInvoker) Invoke(_ context.Context, target Target, method string, payload []byte, md map[string]string) ([]byte, *CallError) {
	f.called = true
	f.got.method = method
	f.got.payload = payload
	f.got.md = md
	f.got.tgt = target
	return f.payload, f.cerr
}

func TestSplitMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		method  string
		service string
		ok      bool
	}{
		{"обычный", "miki.GetProgram", "miki", true},
		{"мультиточечный", "a.b.c", "a", true},
		{"без точки", "miki", "", false},
		{"пустой", "", "", false},
		{"точка в начале", ".miki", "", false},
		{"точка в конце", "miki.", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, ok := SplitMethod(tc.method)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.service, svc)
		})
	}
}

func TestCallSuccessPassthrough(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{tgt: Target{Addr: "mailer", Port: 2233}}
	inv := &fakeInvoker{payload: []byte(`{"ok":true}`)}
	svc := New(router, nil, inv) // allow == nil → default-allow

	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{
		Method:   "mailer.SendTemplate",
		Payload:  []byte(`{"to":"a@b.c"}`),
		Metadata: map[string]string{"X-Request-Id": "r1"},
	}, &res)

	require.NoError(t, err)
	require.Nil(t, res.Error)
	assert.Equal(t, []byte(`{"ok":true}`), res.Payload)
	require.True(t, router.called)
	assert.Equal(t, []string{"mailer"}, router.got)
	require.True(t, inv.called)
	assert.Equal(t, "mailer.SendTemplate", inv.got.method)
	assert.Equal(t, []byte(`{"to":"a@b.c"}`), inv.got.payload)
	assert.Equal(t, map[string]string{"X-Request-Id": "r1"}, inv.got.md)
	assert.Equal(t, "mailer", inv.got.tgt.Addr)
}

func TestCallNilResponse(t *testing.T) {
	t.Parallel()
	svc := New(&fakeRouter{}, nil, &fakeInvoker{})
	err := svc.Call(context.Background(), GenericRequest{Method: "a.B"}, nil)
	require.Error(t, err)
}

func TestCallBadRequest(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"", "nodot", "a.", ".b"} {
		router := &fakeRouter{}
		inv := &fakeInvoker{}
		svc := New(router, nil, inv)

		var res GenericResponse
		err := svc.Call(context.Background(), GenericRequest{Method: method}, &res)

		require.NoError(t, err)
		require.NotNil(t, res.Error)
		assert.Equal(t, CodeBadRequest, res.Error.Code)
		assert.False(t, router.called)
		assert.False(t, inv.called)
	}
}

func TestCallAllowlistDenied(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{}
	inv := &fakeInvoker{}
	allow := &fakeAllow{err: errors.New("method miki.DeleteProgram is not allowed")}
	svc := New(router, allow, inv)

	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{Method: "miki.DeleteProgram"}, &res)

	require.NoError(t, err)
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeAllowlist, res.Error.Code)
	assert.Equal(t, []string{"miki.DeleteProgram"}, allow.got)
	assert.False(t, router.called, "allowlist должен срабатывать до роутинга")
	assert.False(t, inv.called)
}

func TestCallRouteError(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{err: errors.New("unknown service \"ghost\"")}
	inv := &fakeInvoker{}
	svc := New(router, nil, inv)

	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{Method: "ghost.Do"}, &res)

	require.NoError(t, err)
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeRoute, res.Error.Code)
	assert.Contains(t, res.Error.Message, "ghost")
	assert.False(t, inv.called)
}

func TestCallBackendErrorPropagated(t *testing.T) {
	t.Parallel()
	svc := New(&fakeRouter{tgt: Target{Addr: "mailer"}}, nil, &fakeInvoker{
		cerr: &CallError{Code: CodeBackend, Message: "template not found", Target: "mailer:2233"},
	})

	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{Method: "mailer.SendTemplate"}, &res)

	require.NoError(t, err)
	require.NotNil(t, res.Error)
	assert.Equal(t, CodeBackend, res.Error.Code)
	assert.Equal(t, "template not found", res.Error.Message)
	assert.Equal(t, "mailer:2233", res.Error.Target)
}

func TestCallEmptyPayloadAllowed(t *testing.T) {
	t.Parallel()
	inv := &fakeInvoker{}
	svc := New(&fakeRouter{tgt: Target{}}, nil, inv)

	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{Method: "sys.Ping"}, &res)

	require.NoError(t, err)
	require.Nil(t, res.Error)
	assert.Nil(t, res.Payload)
	assert.Nil(t, inv.got.payload)
}

func TestCallUnconfiguredService(t *testing.T) {
	t.Parallel()
	svc := New(nil, nil, &fakeInvoker{})
	var res GenericResponse
	err := svc.Call(context.Background(), GenericRequest{Method: "a.B"}, &res)
	require.Error(t, err)
}
